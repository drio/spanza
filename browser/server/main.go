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

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"tailscale.com/derp"
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

	// Ports for WireGuard and Spanza gateway
	wgPort      = 51820 // WireGuard listens here (UDP)
	gatewayPort = 51821 // Spanza gateway listens here (receives from WireGuard)
)

// Cryptographic keys
// These keys identify the server peer
const (
	// Server's DERP keys (for DERP relay identity)
	serverDERPPrivate = "privkey:a85c6983dd4e96c1e54aed78a21b3e50f26bd2786cbddfb6d01cdd77673bda7d"
	serverDERPPublic  = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"

	// Server's WireGuard keys (for tunnel encryption)
	serverWGPrivate = "087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379"
	serverWGPublic  = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"

	// Browser peer's keys (we need browser's public keys to configure WireGuard peer)
	browserDERPPublic = "nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"
	browserWGPublic   = "c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28"
	browserIP         = "192.168.4.2"
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
	go runSpanzaGateway(ctx)

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
`, serverWGPrivate, wgPort, browserWGPublic, browserIP, gatewayPort)

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

// runSpanzaGateway proxies UDP packets between WireGuard and DERP
// This is the same gateway logic we use in userspace/ustest.go
func runSpanzaGateway(ctx context.Context) {
	prefix := "[gateway]"
	log.Printf("%s Starting Spanza gateway (UDP ↔ DERP)...", prefix)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(serverDERPPrivate)); err != nil {
		log.Fatalf("%s Failed to parse private key: %v", prefix, err)
	}

	// Parse browser peer's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(browserDERPPublic)); err != nil {
		log.Fatalf("%s Failed to parse remote public key: %v", prefix, err)
	}

	// Create UDP listener for WireGuard
	// WireGuard will send packets to this port
	listenAddr := fmt.Sprintf(":%d", gatewayPort)
	udpAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("%s Invalid listen address: %v", prefix, err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("%s Failed to listen on UDP: %v", prefix, err)
	}
	defer udpConn.Close()

	log.Printf("%s Listening on UDP port %d", prefix, gatewayPort)

	// WireGuard endpoint (where to send received DERP packets)
	wgEndpoint := fmt.Sprintf("127.0.0.1:%d", wgPort)
	wgAddr, err := net.ResolveUDPAddr("udp", wgEndpoint)
	if err != nil {
		log.Fatalf("%s Invalid WireGuard endpoint: %v", prefix, err)
	}

	// Create DERP client
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		// Suppress verbose DERP logs
		// Uncomment for debugging:
		// log.Printf("[derp] "+format, args...)
	}

	derpClient, err := derphttp.NewClient(privKey, derpURL, logf, netMon)
	if err != nil {
		log.Fatalf("%s Failed to create DERP client: %v", prefix, err)
	}
	defer derpClient.Close()

	log.Printf("%s Connected to DERP server", prefix)
	log.Printf("%s Gateway ready (WireGuard:%d ↔ DERP)", prefix, wgPort)

	// Goroutine: UDP → DERP
	// Read packets from WireGuard, send to DERP
	go func() {
		buf := make([]byte, 65535)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, _, err := udpConn.ReadFromUDP(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("%s UDP read error: %v", prefix, err)
				continue
			}

			// Send to browser peer via DERP
			if err := derpClient.Send(remotePubKey, buf[:n]); err != nil {
				log.Printf("%s DERP send error: %v", prefix, err)
			}
		}
	}()

	// Goroutine: DERP → UDP
	// Receive packets from DERP, send to WireGuard
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := derpClient.Recv()
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("%s DERP recv error: %v", prefix, err)
				continue
			}

			// Only handle received packets
			switch m := msg.(type) {
			case derp.ReceivedPacket:
				// Send to WireGuard
				_, err := udpConn.WriteToUDP(m.Data, wgAddr)
				if err != nil {
					log.Printf("%s UDP write error: %v", prefix, err)
				}
			}
		}
	}()

	<-ctx.Done()
	log.Printf("%s Gateway shutting down", prefix)
}
