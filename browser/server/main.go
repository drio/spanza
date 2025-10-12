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
	"time"

	"github.com/drio/spanza/gateway"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Network configuration
// This server will be 192.168.4.1, browser peer will be 192.168.4.2
const (
	derpURL = "https://derp.tailscale.com/derp"

	// Server peer IPs
	serverIP = "192.168.4.1"
	dnsIP    = "8.8.8.8"

	// Ports for WireGuard and Spanza gateway
	wgPort      = 51820 // WireGuard listens here (UDP)
	gatewayPort = 51821 // Spanza gateway listens here (receives from WireGuard)
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
	log.Println("Starting WireGuard server peer for browser testing...")

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

	// Start the Spanza gateway
	// This proxies UDP packets from WireGuard to DERP and back
	log.Println("Starting Spanza gateway...")

	// Create UDP listener for gateway
	gatewayUDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", gatewayPort))
	if err != nil {
		log.Fatalf("Failed to resolve gateway UDP address: %v", err)
	}
	gatewayUDPConn, err := net.ListenUDP("udp", gatewayUDPAddr)
	if err != nil {
		log.Fatalf("Failed to create gateway UDP listener: %v", err)
	}
	defer gatewayUDPConn.Close()

	// Start gateway
	go func() {
		cfg := gateway.Config{
			Prefix:          "[gateway]",
			DerpURL:         derpURL,
			PrivKeyStr:      peerServerDERPPrivate,
			RemotePubKeyStr: peerBrowserDERPPublic,
			WGEndpoint:      fmt.Sprintf("127.0.0.1:%d", wgPort),
			Verbose:         true, // Enable verbose logging for server
		}
		if err := gateway.Run(ctx, cfg, gatewayUDPConn); err != nil {
			log.Printf("[gateway] Error: %v", err)
		}
	}()

	// Give gateway a moment to start
	time.Sleep(500 * time.Millisecond)

	// Start the WireGuard peer with HTTP server
	log.Println("Starting WireGuard peer...")
	runWireGuardPeer(ctx)
}

// runWireGuardPeer creates the userspace WireGuard device and HTTP server
func runWireGuardPeer(ctx context.Context) {
	log.Printf("Creating userspace WireGuard device on %s...", serverIP)

	// Create userspace network stack (gvisor netstack)
	// tun: Virtual TUN device for WireGuard to read/write IP packets
	// tnet: Userspace TCP/IP stack - implements standard Go net interfaces
	//       We'll use tnet.ListenTCP() to create our HTTP server
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(serverIP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420, // MTU
	)
	if err != nil {
		log.Fatalf("Failed to create TUN: %v", err)
	}

	// Create WireGuard device
	// This wraps the TUN interface and handles WireGuard protocol:
	// - Encryption/decryption
	// - Handshakes
	// - Peer management
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	// Configure WireGuard
	// This is like running: wg set wg0 private-key ... peer ... allowed-ips ...
	wgConfig := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=%s/32
endpoint=127.0.0.1:%d
persistent_keepalive_interval=25
`, peerServerWGPrivate, wgPort, peerBrowserWGPublic, browserIP, gatewayPort)

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
	log.Printf("  Listening: UDP port %d", wgPort)
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
