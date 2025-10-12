package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/drio/spanza/gateway"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

const (
	derpURL = "https://derp.tailscale.com/derp"

	// IP addresses
	peer1IP = "192.168.4.1"
	peer2IP = "192.168.4.2"
	dnsIP   = "8.8.8.8"

	// Ports
	peer1WGPort      = 51820
	peer1GatewayPort = 51821
	peer2WGPort      = 51822
	peer2GatewayPort = 51823

	// Peer 1 DERP keys
	peer1DERPPrivate = "privkey:a85c6983dd4e96c1e54aed78a21b3e50f26bd2786cbddfb6d01cdd77673bda7d"
	peer1DERPPublic  = "nodekey:4b115ea75d1aeb08d489d9b9015f4b8228a60e1cfe4e231332e29bc4da71f659"

	// Peer 2 DERP keys
	peer2DERPPrivate = "privkey:503685023b6d449ea3ade66f9348778666bf2fae863580e86124e7388b4bc37c"
	peer2DERPPublic  = "nodekey:e3603e7b1d8024bad24da4c413b5989211c4f8e5ead29660f05addaa454e810b"

	// WireGuard keys (same as container setup)
	peer1WGPrivate = "087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379"
	peer1WGPublic  = "f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c"

	peer2WGPrivate = "003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641"
	peer2WGPublic  = "c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28"
)

func main() {
	log.Println("Starting userspace WireGuard + Spanza test...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Channel to signal when peer1 server is ready
	peer1Ready := make(chan struct{})

	// Start Spanza gateways
	// Each peer gets its own gateway with unique ports
	log.Println("Starting Spanza gateways...")

	// Create UDP listener for peer1 gateway
	peer1UDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", peer1GatewayPort))
	if err != nil {
		log.Fatal(err)
	}
	peer1UDPConn, err := net.ListenUDP("udp", peer1UDPAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer peer1UDPConn.Close()

	// Create UDP listener for peer2 gateway
	peer2UDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", peer2GatewayPort))
	if err != nil {
		log.Fatal(err)
	}
	peer2UDPConn, err := net.ListenUDP("udp", peer2UDPAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer peer2UDPConn.Close()

	// Start peer1 gateway
	go func() {
		cfg := gateway.Config{
			Prefix:          "[peer1-gw]",
			DerpURL:         derpURL,
			PrivKeyStr:      peer1DERPPrivate,
			RemotePubKeyStr: peer2DERPPublic,
			WGEndpoint:      fmt.Sprintf("127.0.0.1:%d", peer1WGPort),
			Verbose:         false,
		}
		if err := gateway.Run(ctx, cfg, peer1UDPConn); err != nil {
			log.Printf("[peer1-gw] Error: %v", err)
		}
	}()

	// Start peer2 gateway
	go func() {
		cfg := gateway.Config{
			Prefix:          "[peer2-gw]",
			DerpURL:         derpURL,
			PrivKeyStr:      peer2DERPPrivate,
			RemotePubKeyStr: peer1DERPPublic,
			WGEndpoint:      fmt.Sprintf("127.0.0.1:%d", peer2WGPort),
			Verbose:         false,
		}
		if err := gateway.Run(ctx, cfg, peer2UDPConn); err != nil {
			log.Printf("[peer2-gw] Error: %v", err)
		}
	}()

	// Give gateways a moment to connect to DERP
	time.Sleep(1 * time.Second)

	// Start peer1 (server) in goroutine
	go runPeer1(ctx, peer1Ready)

	// Wait for peer1 to be ready
	<-peer1Ready
	log.Println("Peer1 ready, starting peer2...")

	// Give a moment for WireGuard handshake
	time.Sleep(2 * time.Second)

	// Run peer2 (client) in main goroutine
	runPeer2(ctx)

	log.Println("✅ Test complete!")
}

func runPeer1(ctx context.Context, ready chan struct{}) {
	log.Printf("[peer1] Starting WireGuard + Spanza gateway (%s)...", peer1IP)

	// Create userspace WireGuard interface
	// tun: TUN device for WireGuard to read/write packets
	// tnet: Userspace TCP/IP stack (gvisor netstack) - implements standard Go net interfaces
	//       (DialContext, ListenTCP, etc.) without kernel involvement
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(peer1IP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	// Create WireGuard device that wraps the TUN interface
	// The device handles WireGuard protocol: encryption/decryption, handshakes, peer management
	// It reads plaintext IP packets from tun, encrypts them, sends via UDP
	// and receives encrypted UDP, decrypts them, writes back to tun
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	// Configure WireGuard to point to local Spanza gateway
	wgConfig := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=%s/32
endpoint=127.0.0.1:%d
persistent_keepalive_interval=25
`, peer1WGPrivate, peer1WGPort, peer2WGPublic, peer2IP, peer1GatewayPort)

	err = dev.IpcSet(wgConfig)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("[peer1] WireGuard interface up")

	// Start HTTP server on WireGuard network
	log.Printf("[peer1] Starting HTTP server on %s:80...", peer1IP)

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		log.Panicln(err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("[peer1] HTTP request from %s", request.RemoteAddr)
		io.WriteString(writer, "Hello from peer1 via DERP!")
	})

	log.Println("[peer1] Ready!")
	close(ready)

	// Serve HTTP
	srv := &http.Server{}
	go func() {
		<-ctx.Done()
		srv.Close()
		listener.Close()
	}()

	err = srv.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		log.Printf("[peer1] Server error: %v", err)
	}
}

func runPeer2(ctx context.Context) {
	log.Printf("[peer2] Starting WireGuard + Spanza gateway (%s)...", peer2IP)

	// Create userspace WireGuard interface
	// tun: TUN device for WireGuard to read/write packets
	// tnet: Userspace TCP/IP stack (gvisor netstack) - implements standard Go net interfaces
	//       (DialContext, ListenTCP, etc.) without kernel involvement
	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr(peer2IP)},
		[]netip.Addr{netip.MustParseAddr(dnsIP)},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	// Create WireGuard device that wraps the TUN interface
	// The device handles WireGuard protocol: encryption/decryption, handshakes, peer management
	// It reads plaintext IP packets from tun, encrypts them, sends via UDP
	// and receives encrypted UDP, decrypts them, writes back to tun
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	// Configure WireGuard to point to local Spanza gateway
	wgConfig := fmt.Sprintf(`private_key=%s
listen_port=%d
public_key=%s
allowed_ip=0.0.0.0/0
endpoint=127.0.0.1:%d
`, peer2WGPrivate, peer2WGPort, peer1WGPublic, peer2GatewayPort)

	err = dev.IpcSet(wgConfig)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("[peer2] WireGuard interface up")

	// Give WireGuard handshake time to complete
	log.Println("[peer2] Waiting for handshake...")
	time.Sleep(3 * time.Second)

	// Make HTTP request to peer1
	log.Println("[peer2] Sending HTTP request to peer1...")

	client := http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("http://%s/", peer1IP))
	if err != nil {
		log.Fatalf("[peer2] HTTP request failed: %v", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("[peer2] Failed to read response: %v", err)
	}

	log.Printf("[peer2] ✅ Response from peer1: %s", string(body))
}
