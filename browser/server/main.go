package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/drio/spanza/wgbind"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

// Network configuration
// This server will be 192.168.4.1, browser peer will be 192.168.4.2
const (
	derpURL = "https://derp.tailscale.com/derp"

	// Server peer IPs
	serverIP = "192.168.4.1"
	dnsIP    = "8.8.8.8"
)

// Cryptographic keys
// These keys identify the peers
const (
	// Server peer's DERP keys (for DERP relay identity)
	peerServerDERPPrivate = "privkey:a85c6983dd4e96c1e54aed78a21b3e50f26bd2786cbddfb6d01cdd77673bda7d"
	peerServerDERPPublic  = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"

	// Server peer's WireGuard keys (for tunnel encryption)
	peerServerWGPrivate = "087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379"
	peerServerWGPublic  = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"

	// Browser peer's keys (remote peer that will connect to us)
	peerBrowserDERPPublic = "nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"
	peerBrowserWGPublic   = "e87a7b47066777b678929a3663be293c5d1c3fa279efd3606b90beb58cc54060"
	browserIP             = "192.168.4.2"
)

func main() {
	log.Println("Starting WireGuard server peer with DerpBind...")
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
	log.Printf("Step 2: Creating userspace network stack on %s...", serverIP)
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(serverIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		log.Fatalf("Failed to create TUN: %v", err)
	}

	// Step 3: Start the WireGuard peer with HTTP server
	log.Println("Step 3: Starting WireGuard peer with DERP transport...")
	runWireGuardPeer(ctx, tun, tnet, derpBind)
}

// createDerpBind creates a DERP client and DerpBind for the server
func createDerpBind() (*wgbind.DerpBind, error) {
	log.Printf("Connecting to DERP server: %s", derpURL)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(peerServerDERPPrivate)); err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse browser's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(peerBrowserDERPPublic)); err != nil {
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

// runWireGuardPeer creates the userspace WireGuard device and HTTP server
func runWireGuardPeer(ctx context.Context, tunDev tun.Device, tnet *netstack.Net, derpBind *wgbind.DerpBind) {
	log.Printf("Creating userspace WireGuard device with DERP transport...")

	// Create WireGuard device using DerpBind (no UDP!)
	// This wraps the TUN interface and handles WireGuard protocol:
	// - Encryption/decryption
	// - Handshakes
	// - Peer management
	// DerpBind uses DERP directly for all communication (like Tailscale in WASM)
	dev := device.NewDevice(tunDev, derpBind, device.NewLogger(device.LogLevelVerbose, "[wg-server] "))

	// Configure WireGuard
	// Note: NO listen_port (we're not using UDP)
	// endpoint is the DERP node key (not IP:port)
	wgConfig := fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s
allowed_ip=0.0.0.0/0
persistent_keepalive_interval=25
`, peerServerWGPrivate, peerBrowserWGPublic, peerBrowserDERPPublic)

	log.Println("Configuring WireGuard peer...")
	if err := dev.IpcSet(wgConfig); err != nil {
		log.Fatalf("Failed to configure WireGuard: %v", err)
	}

	// Bring the WireGuard interface up
	if err := dev.Up(); err != nil {
		log.Fatalf("Failed to bring up WireGuard: %v", err)
	}

	log.Println("✓ WireGuard device is up")
	log.Printf("  Address: %s", serverIP)
	log.Printf("  Transport: DERP (no UDP)")
	log.Printf("  Peer configured: %s", browserIP)

	// Start HTTP server on the userspace network
	// This server is only accessible through the WireGuard tunnel
	log.Printf("Starting HTTP server on %s:80...", serverIP)

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		log.Fatalf("Failed to create listener: %v", err)
	}

	// Simple HTTP handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("HTTP request from %s: %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		response := fmt.Sprintf("Hello from WireGuard server!\n\nYou reached %s through the tunnel.\n", serverIP)
		io.WriteString(w, response)
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Status request from %s", r.RemoteAddr)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok","server":"wireguard","ip":"`+serverIP+`"}`)
	})

	log.Println("✓ HTTP server ready")
	log.Println("")
	log.Println("Server is ready! Browser peer can now connect.")
	log.Println("Try: http://192.168.4.1/ or http://192.168.4.1/status")
	log.Println("")

	// Serve HTTP
	srv := &http.Server{}
	go func() {
		<-ctx.Done()
		srv.Close()
		listener.Close()
		dev.Close()
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP server error: %v", err)
	}
}
