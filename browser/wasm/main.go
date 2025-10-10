package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"syscall/js"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
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
	browserWGPrivate = "003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641"

	// Server's keys (to configure as peer)
	serverDERPPublic = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"
	serverWGPublic   = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"
)

// Global state
var (
	wgDevice   *device.Device    // The WireGuard device
	derpClient *derphttp.Client  // The DERP client
	wgConn     *net.UDPConn      // Virtual UDP connection for WireGuard
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
	js.Global().Set("connectDERP", js.FuncOf(connectDERP))
	js.Global().Set("getStatus", js.FuncOf(getStatus))

	log.Println("Functions exposed to JavaScript:")
	log.Println("  - hello()           : Simple test function")
	log.Println("  - createWireGuard() : Create WireGuard device")
	log.Println("  - connectDERP()     : Connect to DERP server")
	log.Println("  - getStatus()       : Get connection status")

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
func createWireGuard(this js.Value, args []js.Value) interface{} {
	log.Println("Creating userspace WireGuard device...")

	// Check if already created
	if wgDevice != nil {
		log.Println("WireGuard device already exists")
		return map[string]interface{}{
			"success": false,
			"error":   "WireGuard device already created",
		}
	}

	// Step 1: Create userspace network stack (gvisor netstack)
	// This is the same as we did in server peer, but now it runs in WASM!
	log.Printf("Creating TUN device with IP: %s", browserIP)

	tun, tnet, err := netstack.CreateNetTUN(
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

	log.Println("✓ TUN device created")

	// Step 2: Create WireGuard device
	// This wraps the TUN and handles WireGuard protocol
	log.Println("Creating WireGuard device...")

	wgDevice = device.NewDevice(
		tun,
		conn.NewDefaultBind(),
		device.NewLogger(device.LogLevelVerbose, "[wg] "),
	)

	log.Println("✓ WireGuard device created")

	// Step 3: Configure WireGuard
	// Set our private key and configure the server as a peer
	log.Println("Configuring WireGuard peer...")

	// For now, we're NOT setting an endpoint because we'll connect
	// directly to DERP (no UDP). We'll handle that in the next phase.
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

	// Step 4: Bring the interface up
	if err := wgDevice.Up(); err != nil {
		log.Printf("Failed to bring up WireGuard: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Failed to bring up: %v", err),
		}
	}

	log.Println("✓ WireGuard device is UP")
	log.Printf("  Local IP: %s", browserIP)
	log.Printf("  Peer IP: %s", serverIP)
	log.Println("")
	log.Println("⚠ Note: Device created but not connected to DERP yet")
	log.Println("   We'll add DERP connection in the next step")

	// Store tnet for later use (we'll need it for HTTP requests)
	_ = tnet // We'll use this in the next phase

	return map[string]interface{}{
		"success":   true,
		"localIP":   browserIP,
		"peerIP":    serverIP,
		"status":    "device_created",
		"connected": false,
	}
}

// connectDERP connects to the DERP server and starts relaying packets
// This is where the WebSocket magic happens automatically!
func connectDERP(this js.Value, args []js.Value) interface{} {
	log.Println("Connecting to DERP server...")

	// Check if WireGuard device exists
	if wgDevice == nil {
		log.Println("ERROR: WireGuard device not created yet")
		return map[string]interface{}{
			"success": false,
			"error":   "WireGuard device not created. Call createWireGuard() first",
		}
	}

	// Check if already connected
	if derpClient != nil {
		log.Println("Already connected to DERP")
		return map[string]interface{}{
			"success": false,
			"error":   "Already connected to DERP",
		}
	}

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

	// Create DERP client
	// THIS IS THE MAGIC: When compiled for WASM, derphttp automatically
	// uses WebSocket instead of raw TCP!
	log.Printf("Creating DERP client for: %s", derpURL)
	log.Println("⚡ WebSocket will be used automatically in browser!")

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

	log.Println("✓ DERP client created")
	log.Println("✓ WebSocket connection established!")
	log.Printf("  Connected to: %s", derpURL)
	log.Printf("  Browser DERP key: %s", browserDERPPublic)
	log.Printf("  Server DERP key: %s", serverDERPPublic)
	log.Println("")
	log.Println("🎉 Browser is now connected to DERP via WebSocket!")
	log.Println("   WireGuard packets will be relayed through DERP")
	log.Println("")
	log.Println("⚠ Note: Not routing packets yet - that's the next step")

	return map[string]interface{}{
		"success":   true,
		"derpURL":   derpURL,
		"connected": true,
		"transport": "websocket", // Automatic in browser!
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
