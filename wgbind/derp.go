package wgbind

import (
	"context"
	"log"
	"net"
	"net/netip"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/types/key"
)

// DerpBind implements conn.Bind for DERP transport (no UDP).
// This is specifically designed for browser/WASM where UDP sockets aren't available.
//
// Unlike NetstackBind which uses userspace UDP + Gateway, DerpBind communicates
// directly with a DERP server, similar to how Tailscale's MagicSock works in WASM.
type DerpBind struct {
	derpClient   *derphttp.Client
	remotePubKey key.NodePublic

	// Receive channel - packets from DERP are sent here
	// This decouples the blocking derpClient.Recv() from WireGuard's receive loop
	recvCh chan derpPacket

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// Mutex protects closed state and receive loop state
	mu              sync.Mutex
	closed          bool
	recvLoopStarted bool // Track if receive loop has been started
}

var _ conn.Bind = (*DerpBind)(nil)

// derpPacket represents a received packet from DERP
type derpPacket struct {
	data []byte
	from key.NodePublic
}

// DerpEndpoint implements conn.Endpoint for DERP.
// In DERP, endpoints are identified by node public keys, not IP:port addresses.
type DerpEndpoint struct {
	publicKey key.NodePublic
}

var _ conn.Endpoint = (*DerpEndpoint)(nil)

func (e *DerpEndpoint) ClearSrc()           {}
func (e *DerpEndpoint) SrcToString() string { return e.publicKey.ShortString() }
func (e *DerpEndpoint) SrcIP() netip.Addr   { return netip.Addr{} }
func (e *DerpEndpoint) DstToString() string { return e.publicKey.ShortString() }
func (e *DerpEndpoint) DstIP() netip.Addr   { return netip.Addr{} }
func (e *DerpEndpoint) DstToBytes() []byte  { return e.publicKey.AppendTo(nil) }

// NewDerpBind creates a new DERP-based conn.Bind.
//
// Parameters:
//   - client: An active DERP client (already connected or will connect automatically)
//   - remotePubKey: The DERP public key of the remote peer we'll communicate with
//
// The bind starts in a closed state. Call Open() to start receiving packets.
func NewDerpBind(client *derphttp.Client, remotePubKey key.NodePublic) *DerpBind {
	ctx, cancel := context.WithCancel(context.Background())

	bind := &DerpBind{
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
//
// Like Tailscale's MagicSock in WASM mode, we return only a DERP receive function,
// no UDP receive functions.
func (b *DerpBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.closed {
		return nil, 0, conn.ErrBindAlreadyOpen
	}
	b.closed = false

	log.Println("[derpbind] Opening DERP bind...")

	// Start receive loop immediately for WASM compatibility
	// WASM has different goroutine scheduling, so we need the loop running
	// before any sends happen to ensure proper message handling
	if !b.recvLoopStarted {
		b.recvLoopStarted = true
		log.Println("[derpbind] Starting receive loop immediately (WASM compatibility)")
		go b.receiveLoop()
	}

	// Return a single receive function (DERP only, no UDP)
	// WireGuard will call this function to receive packets
	fns := []conn.ReceiveFunc{b.receiveDERP}

	// Return fake port number (like MagicSock does for WASM)
	// WireGuard requires a port number but we don't use UDP
	log.Println("[derpbind] ✓ DERP bind opened with receive loop running")
	return fns, 12345, nil
}

// Close implements conn.Bind.Close
func (b *DerpBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	log.Println("[derpbind] Closing DERP bind...")
	b.closed = true
	b.cancel() // Stop receive loop
	close(b.recvCh)

	return nil
}

// Send implements conn.Bind.Send
// This is called by WireGuard when it wants to send packets.
func (b *DerpBind) Send(buffs [][]byte, ep conn.Endpoint) error {
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
		// This will establish the DERP WebSocket connection if not already connected
		if err := b.derpClient.Send(b.remotePubKey, buff); err != nil {
			// Error already logged by derpClient, just return it
			return err
		}
	}

	return nil
}

// SetMark implements conn.Bind.SetMark
// This is a no-op for DERP (used for routing marks on Linux)
func (b *DerpBind) SetMark(mark uint32) error {
	return nil
}

// BatchSize implements conn.Bind.BatchSize
// Returns the batch size for sending/receiving packets
func (b *DerpBind) BatchSize() int {
	return 1 // DERP sends one packet at a time
}

// ParseEndpoint implements conn.Bind.ParseEndpoint
// WireGuard calls this to parse endpoint strings from configuration.
// For DERP, we always return our single remote endpoint.
func (b *DerpBind) ParseEndpoint(s string) (conn.Endpoint, error) {
	// For simplicity, we just return our single endpoint
	// In a more complex setup, you could parse node key strings here
	return &DerpEndpoint{publicKey: b.remotePubKey}, nil
}

// receiveDERP is the receive function called by WireGuard
// It reads packets from our receive channel.
//
// This is the function returned by Open() that WireGuard will call
// repeatedly to receive packets.
func (b *DerpBind) receiveDERP(buffs [][]byte, sizes []int, eps []conn.Endpoint) (int, error) {
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
		eps[0] = &DerpEndpoint{publicKey: pkt.from}

		return 1, nil
	}
}

// receiveLoop runs in a goroutine and reads packets from DERP
// It feeds received packets into the recvCh channel.
//
// This is the key to making DERP work with WireGuard's blocking receive model:
// - derpClient.Recv() is a blocking call
// - We run it in a goroutine and feed results into a channel
// - receiveDERP() reads from that channel non-blockingly
func (b *DerpBind) receiveLoop() {
	log.Println("[derpbind] Starting DERP receive loop...")
	log.Println("[derpbind] Waiting for browser to initialize WebSocket...")

	// In WASM, give the browser more time to fully initialize
	// Progressive delays: start with longer wait, then retry with backoff
	time.Sleep(2 * time.Second)

	firstConnect := true
	retryCount := 0

	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		// Yield to the JavaScript event loop
		time.Sleep(10 * time.Millisecond)

		msg, err := b.derpClient.Recv()
		if err != nil {
			select {
			case <-b.ctx.Done():
				return
			default:
			}

			retryCount++
			if retryCount == 1 {
				log.Printf("[derpbind] Attempting connection (retry %d)...", retryCount)
			} else if retryCount%2 == 0 {
				log.Printf("[derpbind] Retrying (attempt %d)...", retryCount)
			}

			// Exponential backoff after failed attempts
			// Wait longer between retries to reduce error spam
			if retryCount > 1 {
				backoff := time.Duration(retryCount) * 500 * time.Millisecond
				if backoff > 3*time.Second {
					backoff = 3 * time.Second
				}
				time.Sleep(backoff)
			}
			continue
		}

		// Connection succeeded
		if firstConnect {
			log.Printf("[derpbind] ✓ Connected to DERP after %d attempts", retryCount+1)
			firstConnect = false
		}
		retryCount = 0

		// Handle different DERP message types
		switch m := msg.(type) {
		case derp.ReceivedPacket:
			data := make([]byte, len(m.Data))
			copy(data, m.Data)

			pkt := derpPacket{
				data: data,
				from: m.Source,
			}

			select {
			case b.recvCh <- pkt:
				// Only log first few packets, then be quiet
				if firstConnect {
					log.Printf("[derpbind] Received %d bytes from %s", len(data), m.Source.ShortString())
				}
			case <-b.ctx.Done():
				return
			default:
				log.Println("[derpbind] WARNING: Receive queue full, dropping packet")
			}

		case derp.ServerInfoMessage:
			log.Println("[derpbind] ✓ Received ServerInfo from DERP")

		default:
			// Silently ignore other message types (like KeepAlive)
		}
	}
}
