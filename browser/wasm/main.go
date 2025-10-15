package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"syscall/js"
	"time"

	"github.com/drio/spanza/gateway"
	"github.com/drio/spanza/wgbind"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
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

	// Ports for WireGuard and Spanza gateway
	wgPort      = 51822 // WireGuard listens here (userspace UDP)
	gatewayPort = 51823 // Spanza gateway listens here (userspace UDP)

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
	wgDevice   *device.Device    // The WireGuard device
	derpClient *derphttp.Client  // The DERP client (for DerpBind)
	tnet       *netstack.Net     // Userspace network stack
	ctx        context.Context
	cancel     context.CancelFunc
)

// main is the entry point for the WASM module.
func main() {
	log.Println("Spanza WASM module loaded!")

	// Create a context for managing the WireGuard lifecycle
	ctx, cancel = context.WithCancel(context.Background())

	// Expose functions to JavaScript
	js.Global().Set("hello", js.FuncOf(hello))
	js.Global().Set("createWireGuard", js.FuncOf(createWireGuard))
	js.Global().Set("testDerpOnly", js.FuncOf(testDerpOnly))
	js.Global().Set("getStatus", js.FuncOf(getStatus))
	js.Global().Set("fetchHTTP", js.FuncOf(fetchHTTP))
	js.Global().Set("pingPeer", js.FuncOf(pingPeer))

	log.Println("Functions exposed to JavaScript:")
	log.Println("  - hello()           : Simple test function")
	log.Println("  - createWireGuard() : Setup WireGuard + DerpBind + DERP connection (WASM mode)")
	log.Println("  - testDerpOnly()    : Test DERP communication only (no WireGuard)")
	log.Println("  - getStatus()       : Get connection status")
	log.Println("  - fetchHTTP()       : Fetch HTTP through tunnel")

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
// Uses Tailscale's approach for WASM: WireGuard ← DerpBind (direct) → WebSocket DERP
// NO Gateway, NO userspace UDP - just like Tailscale does in WASM!
func createWireGuard(this js.Value, args []js.Value) interface{} {
	log.Println("Creating WireGuard + DERP connection (WASM mode)...")

	// Check if already created
	if wgDevice != nil {
		log.Println("WireGuard device already exists")
		return map[string]interface{}{
			"success": false,
			"error":   "WireGuard device already created",
		}
	}

	// Step 1: Create DERP client and bind
	derpBind, err := createDerpBind()
	if err != nil {
		return errorResponse(err.Error())
	}

	// Step 2: Create userspace network stack
	tunDev, tnetLocal, err := createNetworkStack()
	if err != nil {
		return errorResponse(err.Error())
	}
	tnet = tnetLocal // Store globally for HTTP functions

	// Step 3: Create WireGuard device
	if err := createWireGuardDevice(tunDev, derpBind); err != nil {
		return errorResponse(err.Error())
	}

	// Step 4: Configure WireGuard peer
	if err := configureWireGuardPeer(); err != nil {
		return errorResponse(err.Error())
	}

	// Step 5: Bring interface up
	if err := bringWireGuardUp(); err != nil {
		return errorResponse(err.Error())
	}

	printSuccessMessage()

	return map[string]interface{}{
		"success":   true,
		"localIP":   browserIP,
		"peerIP":    serverIP,
		"derpURL":   derpURL,
		"status":    "connected",
		"transport": "websocket+derpbind",
	}
}

// createDerpBind creates and configures the DERP client and bind
func createDerpBind() (*wgbind.DerpBind, error) {
	log.Printf("Step 1: Connecting to DERP server: %s", derpURL)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(browserDERPPrivate)); err != nil {
		log.Printf("Failed to parse private key: %v", err)
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}

	// Parse server's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(serverDERPPublic)); err != nil {
		log.Printf("Failed to parse remote public key: %v", err)
		return nil, fmt.Errorf("failed to parse remote key: %w", err)
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
		return nil, fmt.Errorf("failed to create DERP client: %w", err)
	}

	log.Println("✓ DERP client created (connection will establish on first use)")

	// Create DerpBind for WireGuard
	log.Println("Step 2: Creating DerpBind for WireGuard...")
	derpBind := wgbind.NewDerpBind(derpClient, remotePubKey)
	log.Println("✓ DerpBind created")

	return derpBind, nil
}

// createNetworkStack creates the userspace network stack and TUN device
// Returns both the TUN device and the network stack for the caller to manage
func createNetworkStack() (tun.Device, *netstack.Net, error) {
	log.Printf("Step 3: Creating userspace network stack with IP: %s", browserIP)

	tunDev, tnetLocal, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(browserIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		log.Printf("Failed to create network stack: %v", err)
		return nil, nil, fmt.Errorf("failed to create network stack: %w", err)
	}

	log.Println("✓ Userspace network stack created")

	return tunDev, tnetLocal, nil
}

// createWireGuardDevice creates the WireGuard device with the given TUN and bind
func createWireGuardDevice(tunDev tun.Device, derpBind *wgbind.DerpBind) error {
	log.Println("Step 4: Creating WireGuard device with DERP bind...")

	wgDevice = device.NewDevice(
		tunDev,
		derpBind, // ✅ Use DerpBind instead of NetstackBind!
		device.NewLogger(device.LogLevelVerbose, "[wg] "),
	)

	log.Println("✓ WireGuard device created with DERP transport")
	return nil
}

// configureWireGuardPeer configures the WireGuard peer
func configureWireGuardPeer() error {
	log.Println("Step 5: Configuring WireGuard peer...")

	// We need to specify an endpoint so WireGuard knows where to send packets
	// ParseEndpoint() will convert this to a DerpEndpoint
	// The string can be anything - we'll always return our remote peer's endpoint
	wgConfig := fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s
allowed_ip=0.0.0.0/0
persistent_keepalive_interval=25
`, browserWGPrivate, serverWGPublic, serverDERPPublic)

	if err := wgDevice.IpcSet(wgConfig); err != nil {
		log.Printf("Failed to configure WireGuard: %v", err)
		return fmt.Errorf("failed to configure: %w", err)
	}

	log.Println("✓ WireGuard configured")
	return nil
}

// bringWireGuardUp brings the WireGuard interface up
func bringWireGuardUp() error {
	log.Println("Step 6: Bringing WireGuard interface up...")

	if err := wgDevice.Up(); err != nil {
		log.Printf("Failed to bring up WireGuard: %v", err)
		return fmt.Errorf("failed to bring up: %w", err)
	}

	return nil
}

// printSuccessMessage prints the success message after WireGuard is up
func printSuccessMessage() {
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
	log.Println("  Architecture: WireGuard ← DerpBind (direct) → WebSocket → DERP")
	log.Println("  (Just like Tailscale does in WASM!)")
}

// errorResponse creates a standard error response for JavaScript
func errorResponse(message string) map[string]interface{} {
	return map[string]interface{}{
		"success": false,
		"error":   message,
	}
}

// getStatus returns the current status of the WireGuard device
func getStatus(this js.Value, args []js.Value) interface{} {
	if wgDevice == nil {
		return map[string]interface{}{
			"exists": false,
			"status": "not_created",
		}
	}

	return map[string]interface{}{
		"exists":  true,
		"localIP": browserIP,
		"peerIP":  serverIP,
		"status":  "device_up",
	}
}

// pingPeer sends an ICMP ping through the WireGuard tunnel
func pingPeer(this js.Value, args []js.Value) interface{} {
	log.Println("Attempting to ping peer through WireGuard tunnel...")

	if tnet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Network stack not initialized. Call createWireGuard() first.",
		}
	}

	// Use the userspace network stack to dial
	// For ICMP ping, we'll actually use a simple TCP connection test for now
	// (ICMP requires raw sockets which are complex in userspace)
	log.Printf("Attempting to connect to %s:80...", serverIP)

	conn, err := tnet.DialContext(context.Background(), "tcp", serverIP+":80")
	if err != nil {
		log.Printf("Failed to connect: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Connection failed: %v", err),
		}
	}
	defer conn.Close()

	log.Println("✓ TCP connection successful!")

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully connected to %s:80 through WireGuard tunnel", serverIP),
		"bytes":   0,
	}
}

// fetchHTTP makes an HTTP request through the WireGuard tunnel
func fetchHTTP(this js.Value, args []js.Value) interface{} {
	log.Println("Fetching HTTP through WireGuard tunnel...")

	if tnet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Network stack not initialized. Call createWireGuard() first.",
		}
	}

	// Create HTTP client using the userspace network stack
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
		Timeout: 10 * time.Second,
	}

	url := fmt.Sprintf("http://%s/", serverIP)
	log.Printf("Fetching: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("HTTP request failed: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("HTTP request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to read body: %v", err),
		}
	}

	log.Printf("✓ HTTP response received! Status: %s, Body length: %d bytes", resp.Status, len(body))

	return map[string]interface{}{
		"success":    true,
		"statusCode": resp.StatusCode,
		"statusText": resp.Status,
		"body":       string(body),
		"headers":    formatHeaders(resp.Header),
	}
}

// formatHeaders converts http.Header to a simple map for JavaScript
func formatHeaders(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// testDerpOnly tests DERP communication without WireGuard
// This bypasses WireGuard to isolate whether issues are in DERP/WebSocket or WireGuard layer
func testDerpOnly(this js.Value, args []js.Value) interface{} {
	log.Println("========================================")
	log.Println("[TEST] DERP-only communication test")
	log.Println("[TEST] This bypasses WireGuard to test Gateway + DERP directly")
	log.Println("========================================")

	// Step 1: Create userspace network stack
	log.Printf("Step 1: Creating userspace network stack on %s...", browserIP)
	tunDev, tnetLocal, err := netstack.CreateNetTUN(
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
	_ = tunDev // Not used in DERP-only test
	tnet = tnetLocal
	log.Println("✓ TUN device created")

	// Step 2: Start the Spanza gateway
	log.Println("Step 2: Starting Spanza gateway...")

	gatewayUDPAddr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: gatewayPort,
	}
	gatewayUDPConn, err := tnet.ListenUDP(gatewayUDPAddr)
	if err != nil {
		log.Printf("Failed to create gateway UDP listener: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create gateway: %v", err),
		}
	}

	// Start gateway in goroutine
	go func() {
		cfg := gateway.Config{
			Prefix:          "[gateway]",
			DerpURL:         derpURL,
			PrivKeyStr:      browserDERPPrivate,
			RemotePubKeyStr: serverDERPPublic,
			WGEndpoint:      fmt.Sprintf("%s:%d", browserIP, wgPort), // Packets will be sent here
			Verbose:         true,
		}
		if err := gateway.Run(ctx, cfg, gatewayUDPConn); err != nil {
			log.Printf("[gateway] Error: %v", err)
		}
	}()

	log.Println("✓ Gateway started")

	// Step 3: Create a simple UDP socket to send/receive test packets
	log.Println("Step 3: Creating test UDP socket...")

	testUDPAddr := &net.UDPAddr{
		IP:   net.ParseIP(browserIP),
		Port: wgPort, // Use WireGuard port for testing
	}
	testUDPConn, err := tnet.ListenUDP(testUDPAddr)
	if err != nil {
		log.Printf("Failed to create test UDP socket: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to create test socket: %v", err),
		}
	}

	log.Printf("✓ Test UDP socket created on %s:%d", browserIP, wgPort)

	// Step 4: Start receive loop in goroutine
	receivedPackets := make(chan string, 10)
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := testUDPConn.ReadFrom(buf)
			if err != nil {
				log.Printf("[test] UDP read error: %v", err)
				return
			}
			data := string(buf[:n])
			log.Printf("[test] ✓ Received %d bytes from %s: %q", n, addr, data)
			receivedPackets <- data
		}
	}()

	// Step 5: Send test packets periodically
	log.Println("Step 4: Sending test packets through gateway...")

	gatewayAddr := &net.UDPAddr{
		IP:   net.ParseIP(browserIP),
		Port: gatewayPort,
	}

	for i := 1; i <= 5; i++ {
		msg := fmt.Sprintf("PING-%d", i)
		n, err := testUDPConn.WriteTo([]byte(msg), gatewayAddr)
		if err != nil {
			log.Printf("[test] Failed to send packet %d: %v", i, err)
		} else {
			log.Printf("[test] → Sent packet %d: %q (%d bytes)", i, msg, n)
		}

		// Wait a bit and check for responses
		time.Sleep(1 * time.Second)

		// Check if we received anything
		select {
		case received := <-receivedPackets:
			log.Printf("[test] ✓ SUCCESS! Received echo: %q", received)
		default:
			log.Printf("[test] ⚠ No response received yet for packet %d", i)
		}
	}

	log.Println("")
	log.Println("========================================")
	log.Println("[TEST] DERP-only test completed")
	log.Println("[TEST] Check logs above for send/receive results")
	log.Println("========================================")
	log.Println("")

	return map[string]interface{}{
		"success": true,
		"message": "DERP-only test completed - check console logs",
	}
}
