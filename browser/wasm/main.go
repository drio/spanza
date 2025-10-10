package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"syscall/js"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Configuration - same keys as server peer
const (
	// Browser peer network config
	browserIP = "192.168.4.2"
	serverIP  = "192.168.4.1"
	dnsIP     = "8.8.8.8"

	// Browser's WireGuard keys
	browserWGPrivate = "003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641"

	// Server's WireGuard public key (to configure as peer)
	serverWGPublic = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"
)

// Global state
var (
	wgDevice *device.Device // The WireGuard device
	ctx      context.Context
	cancel   context.CancelFunc
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

	log.Println("Functions exposed to JavaScript:")
	log.Println("  - hello()           : Simple test function")
	log.Println("  - createWireGuard() : Create WireGuard device")
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

// getStatus returns the current status of the WireGuard device
func getStatus(this js.Value, args []js.Value) interface{} {
	if wgDevice == nil {
		return map[string]interface{}{
			"exists":    false,
			"status":    "not_created",
			"connected": false,
		}
	}

	return map[string]interface{}{
		"exists":    true,
		"localIP":   browserIP,
		"peerIP":    serverIP,
		"status":    "device_up",
		"connected": false, // Not connected to DERP yet
	}
}
