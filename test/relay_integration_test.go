//go:build integration

package test

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// TestWireGuardDirect tests two WireGuard peers communicating directly
// without relay (baseline test to verify setup works)
func TestWireGuardDirect(t *testing.T) {
	log.Println("Starting direct WireGuard connection test...")

	// Channel to signal when server is ready
	serverReady := make(chan struct{})

	// Start server peer
	go func() {
		tnet := startServerPeer(t)
		startHTTPServer(t, tnet, serverReady)
	}()

	// Wait for server to be ready
	<-serverReady
	t.Log("Server ready, starting client...")

	// Start client peer and make request
	tnet := startClientPeer(t)
	makeHTTPRequest(t, tnet)

	t.Log("✅ Direct WireGuard test passed!")
}

// startServerPeer creates a WireGuard peer acting as server (192.168.4.2)
func startServerPeer(t *testing.T) *netstack.Net {
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
	err = dev.IpcSet(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
listen_port=51822
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.1/32
persistent_keepalive_interval=25
`)
	if err != nil {
		t.Fatalf("Failed to configure server device: %v", err)
	}

	if err := dev.Up(); err != nil {
		t.Fatalf("Failed to bring up server device: %v", err)
	}

	t.Log("[server] WireGuard interface up")
	return tnet
}

// startClientPeer creates a WireGuard peer acting as client (192.168.4.1)
func startClientPeer(t *testing.T) *netstack.Net {
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
	err = dev.IpcSet(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
endpoint=127.0.0.1:51822
`)
	if err != nil {
		t.Fatalf("Failed to configure client device: %v", err)
	}

	if err := dev.Up(); err != nil {
		t.Fatalf("Failed to bring up client device: %v", err)
	}

	t.Log("[client] WireGuard interface up")

	// Give WireGuard handshake time to complete
	time.Sleep(2 * time.Second)

	return tnet
}

// startHTTPServer starts an HTTP server on the given network stack
func startHTTPServer(t *testing.T, tnet *netstack.Net, ready chan struct{}) {
	t.Helper()

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		t.Fatalf("Failed to listen on :80: %v", err)
	}

	http.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		t.Logf("[server] Request from %s", r.RemoteAddr)
		io.WriteString(w, "pong from WireGuard server")
	})

	t.Log("[server] HTTP server listening on 192.168.4.2:80")

	// Signal ready
	close(ready)

	// Serve (blocks)
	srv := &http.Server{}
	go func() {
		time.Sleep(10 * time.Second)
		srv.Close()
		listener.Close()
	}()

	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Logf("[server] HTTP server error: %v", err)
	}
}

// makeHTTPRequest makes an HTTP request through the WireGuard tunnel
func makeHTTPRequest(t *testing.T, tnet *netstack.Net) {
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

	expected := "pong from WireGuard server"
	if string(body) != expected {
		t.Errorf("Expected %q, got %q", expected, string(body))
	}
}
