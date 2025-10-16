package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/drio/spanza/wgbind"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

// Network configuration
// This client will be 192.168.4.2, server peer is 192.168.4.1
const (
	derpURL = "https://derp.tailscale.com/derp"

	// Client peer IPs
	clientIP = "192.168.4.2"
	serverIP = "192.168.4.1"
	dnsIP    = "8.8.8.8"
)

// Cryptographic keys
// These are the same keys as the browser peer
const (
	// Client peer's DERP keys (for DERP relay identity)
	peerClientDERPPrivate = "privkey:503685023b6d449ea3ade66f9348778666bf2fae863580e86124e7388b4bc37c"
	peerClientDERPPublic  = "nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"

	// Client peer's WireGuard keys (for tunnel encryption)
	// Must match browser keys (both are 192.168.4.2)
	peerClientWGPrivate = "10a216bad1190b9ebabb373061bd112a3d27d11ab005c0c5bce05c9c7e8eb85f"
	peerClientWGPublic  = "e87a7b47066777b678929a3663be293c5d1c3fa279efd3606b90beb58cc54060"

	// Server peer's keys (remote peer we connect to)
	peerServerDERPPublic = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"
	peerServerWGPublic   = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"
)

func main() {
	log.Println("Starting native WireGuard client peer for testing...")
	log.Println("This client uses DerpBind (same as WASM) for testing")
	log.Println("")

	// Create a context that we can cancel on shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("\nShutdown signal received, cleaning up...")
		cancel()
	}()

	// Step 1: Create DERP client and DerpBind
	log.Println("Step 1: Creating DERP client and DerpBind...")
	derpBind, err := createDerpBind()
	if err != nil {
		log.Fatalf("Failed to create DerpBind: %v", err)
	}

	// Step 2: Create userspace network stack
	log.Printf("Step 2: Creating userspace network stack on %s...", clientIP)
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(clientIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		log.Fatalf("Failed to create TUN: %v", err)
	}

	// Step 3: Start the WireGuard client with DerpBind
	log.Println("Step 3: Starting WireGuard peer with DERP transport...")
	runWireGuardClient(ctx, tun, tnet, derpBind)
}

// createDerpBind creates a DERP client and DerpBind for native Go
func createDerpBind() (*wgbind.DerpBind, error) {
	log.Printf("Connecting to DERP server: %s", derpURL)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(peerClientDERPPrivate)); err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse server's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(peerServerDERPPublic)); err != nil {
		return nil, fmt.Errorf("failed to parse remote public key: %w", err)
	}

	// Create DERP client
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		log.Printf("[derp] "+format, args...)
	}

	derpClient, err := derphttp.NewClient(privKey, derpURL, logf, netMon)
	if err != nil {
		return nil, fmt.Errorf("failed to create DERP client: %w", err)
	}

	log.Println("✓ DERP client created")

	// Create DerpBind for WireGuard
	derpBind := wgbind.NewDerpBind(derpClient, remotePubKey)
	log.Println("✓ DerpBind created")

	return derpBind, nil
}

// runWireGuardClient creates the userspace WireGuard device and makes HTTP request
func runWireGuardClient(ctx context.Context, tunDev tun.Device, tnet *netstack.Net, derpBind *wgbind.DerpBind) {
	log.Printf("Creating userspace WireGuard device with DERP transport...")

	// Create WireGuard device using DerpBind (no UDP!)
	// This wraps the TUN interface and handles WireGuard protocol:
	// - Encryption/decryption
	// - Handshakes
	// - Peer management
	// DerpBind uses DERP directly for all communication (like Tailscale in WASM)
	dev := device.NewDevice(tunDev, derpBind, device.NewLogger(device.LogLevelSilent, "[wg] "))

	// Configure WireGuard
	// Note: NO listen_port (we're not using UDP)
	// endpoint is the DERP node key (not IP:port)
	wgConfig := fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s
allowed_ip=0.0.0.0/0
persistent_keepalive_interval=25
`, peerClientWGPrivate, peerServerWGPublic, peerServerDERPPublic)

	log.Println("Configuring WireGuard peer...")
	if err := dev.IpcSet(wgConfig); err != nil {
		log.Fatalf("Failed to configure WireGuard: %v", err)
	}

	// Bring the WireGuard interface up
	if err := dev.Up(); err != nil {
		log.Fatalf("Failed to bring up WireGuard: %v", err)
	}

	log.Println("✓ WireGuard device is up")
	log.Printf("  Address: %s", clientIP)
	log.Printf("  Transport: DERP (no UDP)")
	log.Printf("  Peer configured: %s", serverIP)
	log.Println("")

	// Wait for handshake to complete
	log.Println("Waiting for WireGuard handshake to complete...")
	time.Sleep(3 * time.Second)

	// Make HTTP request to server
	log.Println("─────────────────────────────────────────")
	log.Println("Making HTTP request through tunnel...")
	log.Println("─────────────────────────────────────────")

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext, // Routes through WireGuard!
		},
		Timeout: 10 * time.Second,
	}

	targetURL := fmt.Sprintf("http://%s/", serverIP)
	log.Printf("GET %s", targetURL)

	resp, err := client.Get(targetURL)
	if err != nil {
		log.Fatalf("❌ HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response: %v", err)
	}

	log.Println("")
	log.Println("✅ SUCCESS! HTTP response received:")
	log.Println("─────────────────────────────────────────")
	log.Printf("Status: %s", resp.Status)
	log.Printf("Body:\n%s", string(body))
	log.Println("─────────────────────────────────────────")
	log.Println("")
	log.Println("The tunnel is working! Press Ctrl+C to exit.")

	// Keep running until interrupted
	<-ctx.Done()
	dev.Close()
}
