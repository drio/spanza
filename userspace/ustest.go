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

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
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
	go runSpanzaGateway(ctx, "[peer1-gw]", peer1DERPPrivate, peer2DERPPublic,
		fmt.Sprintf(":%d", peer1GatewayPort), fmt.Sprintf("127.0.0.1:%d", peer1WGPort))
	go runSpanzaGateway(ctx, "[peer2-gw]", peer2DERPPrivate, peer1DERPPublic,
		fmt.Sprintf(":%d", peer2GatewayPort), fmt.Sprintf("127.0.0.1:%d", peer2WGPort))

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

// runSpanzaGateway runs the Spanza UDP-to-DERP gateway
func runSpanzaGateway(ctx context.Context, prefix, privKeyStr, remotePubKeyStr, listenAddr, wgEndpoint string) {
	log.Printf("%s Starting Spanza gateway...", prefix)

	// Parse DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(privKeyStr)); err != nil {
		log.Fatalf("%s Failed to parse private key: %v", prefix, err)
	}

	// Parse remote peer's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(remotePubKeyStr)); err != nil {
		log.Fatalf("%s Failed to parse remote public key: %v", prefix, err)
	}

	// Create UDP listener
	listenUDPAddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		log.Fatalf("%s Invalid listen address: %v", prefix, err)
	}

	udpConn, err := net.ListenUDP("udp", listenUDPAddr)
	if err != nil {
		log.Fatalf("%s Failed to listen on UDP: %v", prefix, err)
	}
	defer udpConn.Close()

	// Resolve WireGuard endpoint
	wgAddr, err := net.ResolveUDPAddr("udp", wgEndpoint)
	if err != nil {
		log.Fatalf("%s Invalid WireGuard endpoint: %v", prefix, err)
	}

	// Create DERP client
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		// Suppress verbose DERP logs
	}

	derpClient, err := derphttp.NewClient(privKey, derpURL, logf, netMon)
	if err != nil {
		log.Fatalf("%s Failed to create DERP client: %v", prefix, err)
	}
	defer derpClient.Close()

	log.Printf("%s Gateway connected to DERP", prefix)

	// UDP -> DERP
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
				continue
			}

			if err := derpClient.Send(remotePubKey, buf[:n]); err != nil {
				log.Printf("%s DERP send error: %v", prefix, err)
			}
		}
	}()

	// DERP -> UDP
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

			switch m := msg.(type) {
			case derp.ReceivedPacket:
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
