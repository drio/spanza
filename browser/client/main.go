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
// This client will be 192.168.4.2, server peer is 192.168.4.1
const (
	derpURL = "https://derp.tailscale.com/derp"

	// Client peer IPs
	clientIP = "192.168.4.2"
	serverIP = "192.168.4.1"
	dnsIP    = "8.8.8.8"

	// Ports for WireGuard and Spanza gateway
	wgPort      = 51822 // WireGuard listens here (UDP)
	gatewayPort = 51823 // Spanza gateway listens here (receives from WireGuard)
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
	log.Println("This client uses the same configuration as the browser peer")
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

	// Start the Spanza gateway
	// This proxies UDP packets from WireGuard to DERP and back
	log.Println("Starting Spanza gateway...")
	go runSpanzaGateway(ctx)

	// Give gateway a moment to start
	time.Sleep(500 * time.Millisecond)

	// Start the WireGuard peer
	log.Println("Starting WireGuard peer...")
	runWireGuardClient(ctx)
}

// runWireGuardClient creates the userspace WireGuard device and makes HTTP request
func runWireGuardClient(ctx context.Context) {
	log.Printf("Creating userspace WireGuard device on %s...", clientIP)

	// Create userspace network stack (gvisor netstack)
	// tun: Virtual TUN device for WireGuard to read/write IP packets
	// tnet: Userspace TCP/IP stack - implements standard Go net interfaces
	//       We'll use tnet.DialContext() to make HTTP requests
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(clientIP)},
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
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelVerbose, "[wg] "))

	// Configure WireGuard
	// This is like running: wg set wg0 private-key ... peer ... allowed-ips ...
	wgConfig := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=%s/32
endpoint=127.0.0.1:%d
persistent_keepalive_interval=25
`, peerClientWGPrivate, wgPort, peerServerWGPublic, serverIP, gatewayPort)

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
	log.Printf("  Listening: UDP port %d", wgPort)
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

// runSpanzaGateway proxies UDP packets between WireGuard and DERP
// This is the same gateway logic as in server and ustest
func runSpanzaGateway(ctx context.Context) {
	prefix := "[gateway]"
	log.Printf("%s Starting Spanza gateway (UDP ↔ DERP)...", prefix)

	// Parse our DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(peerClientDERPPrivate)); err != nil {
		log.Fatalf("%s Failed to parse private key: %v", prefix, err)
	}

	// Parse server peer's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(peerServerDERPPublic)); err != nil {
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

			// Send to server peer via DERP
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
