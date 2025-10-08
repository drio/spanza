//go:build integration

package test

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/drio/spanza/relay"
	"github.com/drio/spanza/server"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// TestWireGuardWithRelay tests two WireGuard peers communicating through
// the relay server (UDP-only, no client sidecar)
func TestWireGuardWithRelay(t *testing.T) {
	log.Println("Starting WireGuard with relay test...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Start relay server (port 51820)
	t.Log("Starting relay server on :51820...")
	relayServer := startRelayServer(t, ctx, ":51820")
	defer relayServer.Close()

	// Give relay server time to start
	time.Sleep(500 * time.Millisecond)

	// Channel to signal when server is ready
	serverReady := make(chan struct{})

	// 2. Start peer2 (HTTP server) - connects to relay server
	go func() {
		tnet := startServerPeerViaRelay(t)
		startHTTPServerRelay(t, tnet, serverReady)
	}()

	// Wait for HTTP server to be ready
	<-serverReady
	t.Log("HTTP server ready, starting client...")

	// 3. Start peer1 (HTTP client) - connects to relay server
	tnet := startClientPeerViaRelay(t)

	// 4. Make HTTP request through the relay
	makeHTTPRequestRelay(t, tnet)

	t.Log("✅ Relay test passed!")
}

// startRelayServer creates and starts the relay server
func startRelayServer(t *testing.T, ctx context.Context, addr string) *server.Server {
	t.Helper()

	registry := relay.NewRegistry()
	processor := relay.NewProcessor(registry)

	cfg := &server.ServerConfig{
		UDPAddr:   addr,
		Registry:  registry,
		Processor: processor,
	}

	srv, err := server.NewServer(cfg)
	if err != nil {
		t.Fatalf("Failed to create relay server: %v", err)
	}

	// Run server in background
	go func() {
		if err := srv.Run(ctx); err != nil && err != context.Canceled {
			t.Logf("Relay server error: %v", err)
		}
	}()

	return srv
}

// startServerPeerViaRelay creates peer2 that connects through relay client
func startServerPeerViaRelay(t *testing.T) *netstack.Net {
	t.Helper()

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		t.Fatalf("Failed to create server TUN: %v", err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	// Peer2 uses ephemeral port, sends initiations to relay
	err = dev.IpcSet(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.1/32
endpoint=127.0.0.1:51820
persistent_keepalive_interval=1
`)
	if err != nil {
		t.Fatalf("Failed to configure server device: %v", err)
	}

	if err := dev.Up(); err != nil {
		t.Fatalf("Failed to bring up server device: %v", err)
	}

	t.Log("[peer2] WireGuard interface up, ephemeral port, endpoint=127.0.0.1:51820")
	return tnet
}

// startClientPeerViaRelay creates peer1 that connects directly to relay server
func startClientPeerViaRelay(t *testing.T) *netstack.Net {
	t.Helper()

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.1")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		t.Fatalf("Failed to create client TUN: %v", err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	// Peer1 uses ephemeral port, sends initiations to relay
	err = dev.IpcSet(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
endpoint=127.0.0.1:51820
persistent_keepalive_interval=1
`)
	if err != nil {
		t.Fatalf("Failed to configure client device: %v", err)
	}

	if err := dev.Up(); err != nil {
		t.Fatalf("Failed to bring up client device: %v", err)
	}

	t.Log("[peer1] WireGuard interface up, ephemeral port, endpoint=127.0.0.1:51820")

	// Give WireGuard handshake time to complete through relay
	time.Sleep(3 * time.Second)

	return tnet
}

// startHTTPServerRelay starts an HTTP server on the given network stack
func startHTTPServerRelay(t *testing.T, tnet *netstack.Net, ready chan struct{}) {
	t.Helper()

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		t.Fatalf("Failed to listen on :80: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[server] Request from %s", r.RemoteAddr)
		io.WriteString(w, "pong from WireGuard server via relay")
	})

	t.Log("[server] HTTP server listening on 192.168.4.2:80")

	// Signal ready
	close(ready)

	// Serve (blocks)
	srv := &http.Server{Handler: mux}
	go func() {
		time.Sleep(10 * time.Second)
		srv.Close()
		listener.Close()
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Logf("[server] HTTP server error: %v", err)
	}
}

// makeHTTPRequestRelay makes an HTTP request through the WireGuard tunnel
func makeHTTPRequestRelay(t *testing.T, tnet *netstack.Net) {
	t.Helper()

	client := http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("http://192.168.4.2/test")
	if err != nil {
		t.Fatalf("Failed to make HTTP request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	t.Logf("[client] Response: %s", string(body))

	expected := "pong from WireGuard server via relay"
	if string(body) != expected {
		t.Errorf("Expected %q, got %q", expected, string(body))
	}
}
