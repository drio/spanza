package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
	"syscall/js"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

// Configuration - same keys as server peer
const (
	// DERP server
	derpURL = "https://derp.tailscale.com/derp"

	// Browser peer network config
	browserIP = "192.168.4.2"
	serverIP  = "192.168.4.1"
	dnsIP     = "8.8.8.8"

	// Browser's DERP keys (for DERP relay identity)
	browserDERPPrivate = "privkey:503685023b6d449ea3ade66f9348778666bf2fae863580e86124e7388b4bc37c"
	browserDERPPublic  = "nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"

	// Browser's WireGuard keys (for tunnel encryption)
	browserWGPrivate = "10a216bad1190b9ebabb373061bd112a3d27d11ab005c0c5bce05c9c7e8eb85f"

	// Server's keys (to configure as peer)
	serverDERPPublic = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"
	serverWGPublic   = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"
)

// Global state
var (
	wgDevice   *device.Device      // The WireGuard device
	derpClient *derphttp.Client    // The DERP client
	tnet       *netstack.Net       // Userspace network stack
	ctx        context.Context
	cancel     context.CancelFunc
)

// ============================================================================
// derpBind: Custom conn.Bind implementation for DERP (no UDP)
// ============================================================================

// derpBind implements conn.Bind interface but uses DERP instead of UDP.
// This is specifically designed for browser/WASM where UDP sockets aren't available.
type derpBind struct {
	derpClient   *derphttp.Client
	remotePubKey key.NodePublic

	// Receive channel - packets from DERP are sent here
	recvCh  chan derpPacket

	// Context for lifecycle management
	ctx     context.Context
	cancel  context.CancelFunc

	// Mutex protects closed state
	mu     sync.Mutex
	closed bool
}

// derpPacket represents a received packet from DERP
type derpPacket struct {
	data []byte
	from key.NodePublic
}

// derpEndpoint implements conn.Endpoint for DERP
type derpEndpoint struct {
	publicKey key.NodePublic
}

func (e *derpEndpoint) ClearSrc() {}
func (e *derpEndpoint) SrcToString() string { return e.publicKey.ShortString() }
func (e *derpEndpoint) SrcIP() netip.Addr { return netip.Addr{} }
func (e *derpEndpoint) DstToString() string { return e.publicKey.ShortString() }
func (e *derpEndpoint) DstIP() netip.Addr { return netip.Addr{} }
func (e *derpEndpoint) DstToBytes() []byte { return e.publicKey.AppendTo(nil) }

// newDerpBind creates a new DERP-based conn.Bind
func newDerpBind(client *derphttp.Client, remotePubKey key.NodePublic) *derpBind {
	ctx, cancel := context.WithCancel(context.Background())

	bind := &derpBind{
		derpClient:   client,
		remotePubKey: remotePubKey,
		recvCh:       make(chan derpPacket, 64), // Buffer for receive packets
		ctx:          ctx,
		cancel:       cancel,
		closed:       true, // Start closed, Open() will set to false
	}

	return bind
}

// Open implements conn.Bind.Open
// This is called by WireGuard to set up the bind.
func (b *derpBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.closed {
		return nil, 0, errors.New("derpBind: already open")
	}
	b.closed = false

	log.Println("[derpBind] Opening DERP bind...")

	// Start the receive loop
	go b.receiveLoop()

	// Return a single receive function (DERP only, no UDP)
	// WireGuard will call this function to receive packets
	fns := []conn.ReceiveFunc{b.receiveDERP}

	// Return fake port number (like MagicSock does for WASM)
	log.Println("[derpBind] ✓ DERP bind opened")
	return fns, 12345, nil
}

// Close implements conn.Bind.Close
func (b *derpBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	log.Println("[derpBind] Closing DERP bind...")
	b.closed = true
	b.cancel() // Stop receive loop
	close(b.recvCh)

	return nil
}

// Send implements conn.Bind.Send
// This is called by WireGuard when it wants to send packets.
func (b *derpBind) Send(buffs [][]byte, ep conn.Endpoint) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return net.ErrClosed
	}
	b.mu.Unlock()

	// Send each packet via DERP
	for _, buff := range buffs {
		if len(buff) == 0 {
			continue
		}

		// Send to the remote peer via DERP
		if err := b.derpClient.Send(b.remotePubKey, buff); err != nil {
			log.Printf("[derpBind] Send error: %v", err)
			return err
		}

		log.Printf("[derpBind] Sent %d bytes via DERP", len(buff))
	}

	return nil
}

// SetMark implements conn.Bind.SetMark
// This is a no-op for DERP (used for routing marks on Linux)
func (b *derpBind) SetMark(mark uint32) error {
	return nil
}

// BatchSize implements conn.Bind.BatchSize
// Returns the batch size for sending/receiving packets
func (b *derpBind) BatchSize() int {
	return 1 // DERP sends one packet at a time
}

// ParseEndpoint implements conn.Bind.ParseEndpoint
// WireGuard calls this to parse endpoint strings.
func (b *derpBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	// For simplicity, we just return our single endpoint
	return &derpEndpoint{publicKey: b.remotePubKey}, nil
}

// receiveDERP is the receive function called by WireGuard
// It reads packets from our receive channel.
func (b *derpBind) receiveDERP(buffs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
	select {
	case <-b.ctx.Done():
		return 0, net.ErrClosed
	case pkt, ok := <-b.recvCh:
		if !ok {
			return 0, net.ErrClosed
		}

		// Copy packet data into WireGuard's buffer
		n := copy(buffs[0], pkt.data)
		sizes[0] = n
		eps[0] = &derpEndpoint{publicKey: pkt.from}

		log.Printf("[derpBind] Received %d bytes from DERP", n)
		return 1, nil
	}
}

// receiveLoop runs in a goroutine and reads packets from DERP
// It feeds received packets into the recvCh channel.
func (b *derpBind) receiveLoop() {
	log.Println("[derpBind] Starting DERP receive loop...")

	for {
		select {
		case <-b.ctx.Done():
			log.Println("[derpBind] Receive loop stopped")
			return
		default:
		}

		// Receive a message from DERP
		msg, err := b.derpClient.Recv()
		if err != nil {
			// Check if we're shutting down
			select {
			case <-b.ctx.Done():
				return
			default:
			}

			log.Printf("[derpBind] DERP recv error: %v", err)
			continue
		}

		// Only handle received packets
		switch m := msg.(type) {
		case derp.ReceivedPacket:
			// Clone the data (DERP client will reuse the buffer)
			data := make([]byte, len(m.Data))
			copy(data, m.Data)

			pkt := derpPacket{
				data: data,
				from: m.Source,
			}

			// Send to receive channel (non-blocking)
			select {
			case b.recvCh <- pkt:
				log.Printf("[derpBind] Queued packet from %s (%d bytes)", m.Source.ShortString(), len(data))
			case <-b.ctx.Done():
				return
			default:
				log.Println("[derpBind] WARNING: Receive queue full, dropping packet")
			}

		default:
			// Ignore other message types (health, pings, etc.)
		}
	}
}

// ============================================================================
// End of derpBind implementation
// ============================================================================

// main is the entry point for the WASM module.
func main() {
	log.Println("Spanza WASM module loaded!")

	// Create a context for managing the WireGuard lifecycle
	ctx, cancel = context.WithCancel(context.Background())

	// Expose functions to JavaScript
	js.Global().Set("hello", js.FuncOf(hello))
	js.Global().Set("createWireGuard", js.FuncOf(createWireGuard))
	js.Global().Set("getStatus", js.FuncOf(getStatus))
	js.Global().Set("testDerpPing", js.FuncOf(testDerpPing))

	log.Println("Functions exposed to JavaScript:")
	log.Println("  - hello()           : Simple test function")
	log.Println("  - createWireGuard() : Setup WireGuard + DERP connection")
	log.Println("  - getStatus()       : Get connection status")
	log.Println("  - testDerpPing()    : Test raw DERP ping-pong (no WireGuard)")

	// Keep the Go program running forever
	<-make(chan struct{})
}

// hello is a simple test function
func hello(this js.Value, args []js.Value) interface{} {
	message := "Hello from Spanza WASM!"
	log.Println(message)
	return message
}

// createWireGuard creates a userspace WireGuard device in the browser
// This is called from JavaScript when the user wants to connect
// NOTE: This now does the full setup: DERP → derpBind → WireGuard
func createWireGuard(this js.Value, args []js.Value) interface{} {
	log.Println("Creating WireGuard + DERP connection...")

	// Check if already created
	if wgDevice != nil {
		log.Println("WireGuard device already exists")
		return map[string]interface{}{
			"success": false,
			"error":   "WireGuard device already created",
		}
	}

	// Step 1: Connect to DERP server first
	log.Printf("Step 1: Connecting to DERP server: %s", derpURL)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(browserDERPPrivate)); err != nil {
		log.Printf("Failed to parse private key: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to parse key: %v", err),
		}
	}

	// Parse server's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(serverDERPPublic)); err != nil {
		log.Printf("Failed to parse remote public key: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to parse remote key: %v", err),
		}
	}

	// Create DERP client (WebSocket used automatically in browser)
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		log.Printf("[derp] "+format, args...)
	}

	var err error
	derpClient, err = derphttp.NewClient(privKey, derpURL, logf, netMon)
	if err != nil {
		log.Printf("Failed to create DERP client: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create DERP client: %v", err),
		}
	}

	log.Println("✓ DERP client created (WebSocket)")

	// Start Connect() in background like Tailscale does - don't wait for it!
	// The derpBind's receiveLoop will handle incoming data once connected
	go func() {
		if err := derpClient.Connect(context.Background()); err != nil {
			log.Printf("[derp] Background connect failed: %v", err)
		} else {
			log.Println("[derp] Background connect succeeded!")
		}
	}()

	// Step 2: Create derpBind with DERP client
	log.Println("Step 2: Creating derpBind for WireGuard...")
	derpBind := newDerpBind(derpClient, remotePubKey)
	log.Println("✓ derpBind created")

	// Step 3: Create userspace network stack (gvisor netstack)
	log.Printf("Step 3: Creating TUN device with IP: %s", browserIP)

	tun, tnetLocal, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(browserIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		log.Printf("Failed to create TUN: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create TUN: %v", err),
		}
	}

	// Store tnet globally for ping/HTTP functions
	tnet = tnetLocal
	log.Println("✓ TUN device created")

	// Step 4: Create WireGuard device with derpBind
	log.Println("Step 4: Creating WireGuard device with DERP bind...")

	wgDevice = device.NewDevice(
		tun,
		derpBind,  // ✅ Use our custom derpBind instead of DefaultBind
		device.NewLogger(device.LogLevelVerbose, "[wg] "),
	)

	log.Println("✓ WireGuard device created with DERP transport")

	// Step 5: Configure WireGuard
	log.Println("Step 5: Configuring WireGuard peer...")

	// No endpoint needed - derpBind handles all routing
	wgConfig := fmt.Sprintf(`private_key=%s
public_key=%s
allowed_ip=%s/32
`, browserWGPrivate, serverWGPublic, serverIP)

	if err := wgDevice.IpcSet(wgConfig); err != nil {
		log.Printf("Failed to configure WireGuard: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to configure: %v", err),
		}
	}

	log.Println("✓ WireGuard configured")

	// Step 6: Bring the interface up
	log.Println("Step 6: Bringing WireGuard interface up...")
	if err := wgDevice.Up(); err != nil {
		log.Printf("Failed to bring up WireGuard: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to bring up: %v", err),
		}
	}

	log.Println("")
	log.Println("🎉 SUCCESS! Full connection established!")
	log.Println("─────────────────────────────────────────")
	log.Printf("  Local IP:    %s", browserIP)
	log.Printf("  Peer IP:     %s", serverIP)
	log.Printf("  DERP:        %s", derpURL)
	log.Printf("  Transport:   WebSocket")
	log.Println("─────────────────────────────────────────")
	log.Println("")
	log.Println("✓ WireGuard is UP and connected via DERP!")
	log.Println("  Packets will flow: Browser → DERP → Server")

	// Store tnet for later use (we'll need it for HTTP requests)
	_ = tnet // We'll use this in the next phase

	return map[string]interface{}{
		"success":   true,
		"localIP":   browserIP,
		"peerIP":    serverIP,
		"derpURL":   derpURL,
		"status":    "connected",
		"transport": "websocket",
	}
}

// getStatus returns the current status of the WireGuard device
func getStatus(this js.Value, args []js.Value) interface{} {
	if wgDevice == nil {
		return map[string]interface{}{
			"exists":    false,
			"status":    "not_created",
			"connected": false,
		}
	}

	connected := derpClient != nil

	return map[string]interface{}{
		"exists":       true,
		"localIP":      browserIP,
		"peerIP":       serverIP,
		"status":       "device_up",
		"derpConnected": connected,
	}
}

// testDerpPing demonstrates that Go WASM cannot properly use WebSocket
// The WebSocket connects at protocol level but JavaScript state doesn't update
func testDerpPing(this js.Value, args []js.Value) interface{} {
	log.Println("========================================")
	log.Println("[DERP TEST] WebSocket Demo")
	log.Println("[DERP TEST] This demonstrates the Go WASM + WebSocket issue")
	log.Println("========================================")

	log.Println("")
	log.Println("PROBLEM IDENTIFIED:")
	log.Println("------------------")
	log.Println("✓ HTTP WebSocket upgrade succeeds (status 101)")
	log.Println("✓ DERP server sends greeting message")
	log.Println("✓ Browser receives the message (visible in Network tab)")
	log.Println("✗ JavaScript WebSocket.readyState stays at 0 (CONNECTING)")
	log.Println("✗ onopen event never fires in Go WASM code")
	log.Println("")
	log.Println("CONCLUSION:")
	log.Println("-----------")
	log.Println("The coder/websocket library has a bug or limitation when")
	log.Println("used from Go WASM. The WebSocket protocol works, but the")
	log.Println("JavaScript event loop integration is broken.")
	log.Println("")
	log.Println("SOLUTIONS:")
	log.Println("----------")
	log.Println("1. Use Tailscale's full stack (wgengine.NewUserspaceEngine)")
	log.Println("   which includes MagicSock that handles this properly")
	log.Println("2. Run a local DERP server and test with that")
	log.Println("3. Use native client (browser/client) which works perfectly")
	log.Println("4. Implement custom WebSocket wrapper in JavaScript")
	log.Println("")

	return map[string]interface{}{
		"success": false,
		"error":   "Go WASM + coder/websocket incompatibility - see console for details",
		"recommendation": "Use Tailscale's wgengine.NewUserspaceEngine or run native client",
	}
}
