package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"strings"
	"syscall/js"
	"time"

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
	js.Global().Set("getStatus", js.FuncOf(getStatus))
	js.Global().Set("fetchHTTP", js.FuncOf(fetchHTTP))
	js.Global().Set("pingPeer", js.FuncOf(pingPeer))

	log.Println("Functions exposed to JavaScript:")
	log.Println("  - hello()           : Simple test function")
	log.Println("  - createWireGuard() : Setup WireGuard + DerpBind + DERP connection")
	log.Println("  - getStatus()       : Get connection status")
	log.Println("  - fetchHTTP()       : Fetch HTTP through tunnel")
	log.Println("  - pingPeer()        : Test connection to peer")

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

	// Step 6: Wait for handshake to complete
	waitForHandshake()

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
	log.Printf("→ Connecting to DERP server: %s", derpURL)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(browserDERPPrivate)); err != nil {
		return nil, fmt.Errorf("failed to parse key: %w", err)
	}

	// Parse server's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(serverDERPPublic)); err != nil {
		return nil, fmt.Errorf("failed to parse remote key: %w", err)
	}

	// Create DERP client (WebSocket used automatically in browser)
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		// Suppress most DERP logging - retries are normal during connection
		// Only log critical errors, not routine connection attempts
		msg := fmt.Sprintf(format, args...)
		if strings.Contains(msg, "context deadline exceeded") {
			// WebSocket timeout during connection - normal, suppress
			return
		}
		if strings.Contains(msg, "error") || strings.Contains(msg, "failed") {
			log.Printf("[derp] "+format, args...)
		}
	}

	var err error
	derpClient, err = derphttp.NewClient(privKey, derpURL, logf, netMon)
	if err != nil {
		return nil, fmt.Errorf("failed to create DERP client: %w", err)
	}

	// In WASM/browser, WebSocket connections take longer to establish
	// Use a 30-second timeout instead of the default 10 seconds
	derpClient.BaseContext = func() context.Context {
		ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
		return ctx
	}

	// In WASM/browser, we need to use http.DefaultClient for WebSocket to work
	derpClient.TLSConfig = nil // Use browser's TLS

	// Create DerpBind for WireGuard
	derpBind := wgbind.NewDerpBind(derpClient, remotePubKey)
	log.Println("✓ DERP client and DerpBind created")

	return derpBind, nil
}

// createNetworkStack creates the userspace network stack and TUN device
// Returns both the TUN device and the network stack for the caller to manage
func createNetworkStack() (tun.Device, *netstack.Net, error) {
	log.Printf("→ Creating network stack (IP: %s)", browserIP)

	tunDev, tnetLocal, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(browserIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create network stack: %w", err)
	}

	log.Println("✓ Network stack created")

	return tunDev, tnetLocal, nil
}

// createWireGuardDevice creates the WireGuard device with the given TUN and bind
func createWireGuardDevice(tunDev tun.Device, derpBind *wgbind.DerpBind) error {
	log.Println("→ Creating WireGuard device...")

	wgDevice = device.NewDevice(
		tunDev,
		derpBind,
		device.NewLogger(device.LogLevelSilent, "[wg] "),
	)

	log.Println("✓ WireGuard device created")
	return nil
}

// configureWireGuardPeer configures the WireGuard peer
func configureWireGuardPeer() error {
	log.Println("→ Configuring WireGuard peer...")

	wgConfig := fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s
allowed_ip=0.0.0.0/0
persistent_keepalive_interval=25
`, browserWGPrivate, serverWGPublic, serverDERPPublic)

	if err := wgDevice.IpcSet(wgConfig); err != nil {
		return fmt.Errorf("failed to configure: %w", err)
	}

	log.Println("✓ Peer configured")
	return nil
}

// bringWireGuardUp brings the WireGuard interface up
func bringWireGuardUp() error {
	log.Println("→ Starting WireGuard...")

	if err := wgDevice.Up(); err != nil {
		return fmt.Errorf("failed to bring up: %w", err)
	}

	log.Println("✓ WireGuard is UP")
	return nil
}

// waitForHandshake waits for the WireGuard handshake to complete
func waitForHandshake() {
	log.Println("→ Waiting for WireGuard handshake...")
	log.Println("   (Make sure the server is running first!)")

	// Give WireGuard time to complete the handshake
	// The handshake involves:
	// 1. Browser sends initiation packet via DERP
	// 2. Server responds via DERP
	// 3. Both sides derive session keys
	// In WASM with DERP relay, this can take 5-10 seconds
	for i := 0; i < 8; i++ {
		time.Sleep(1 * time.Second)
		if i == 3 {
			log.Println("   Still waiting... (handshake packets traveling via DERP)")
		}
	}

	log.Println("✓ Handshake wait complete")
}

// printSuccessMessage prints the success message after WireGuard is up
func printSuccessMessage() {
	log.Println("")
	log.Println("🎉 Tunnel ready!")
	log.Printf("  Local: %s → Peer: %s", browserIP, serverIP)
	log.Printf("  Transport: DERP via WebSocket")
	log.Println("")
	log.Println("You can now use fetchHTTP() or pingPeer() to test the tunnel")
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
	if tnet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Network stack not initialized. Call createWireGuard() first.",
		}
	}

	log.Printf("→ Testing connection to %s:80...", serverIP)

	conn, err := tnet.DialContext(context.Background(), "tcp", serverIP+":80")
	if err != nil {
		log.Printf("✗ Connection failed: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Connection failed: %v", err),
		}
	}
	defer conn.Close()

	log.Println("✓ Connection successful!")

	return map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Successfully connected to %s:80", serverIP),
		"bytes":   0,
	}
}

// fetchHTTP makes an HTTP request through the WireGuard tunnel
func fetchHTTP(this js.Value, args []js.Value) interface{} {
	if tnet == nil {
		return map[string]interface{}{
			"success": false,
			"error":   "Network stack not initialized. Call createWireGuard() first.",
		}
	}

	url := fmt.Sprintf("http://%s/", serverIP)
	log.Printf("→ Fetching %s...", url)

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
		Timeout: 10 * time.Second,
	}

	resp, err := httpClient.Get(url)
	if err != nil {
		log.Printf("✗ Request failed: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("HTTP request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to read body: %v", err),
		}
	}

	log.Printf("✓ Received %s (%d bytes)", resp.Status, len(body))

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
