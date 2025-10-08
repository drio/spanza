package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func main() {
	log.Println("Starting combined WireGuard test...")

	// Channel to signal when server is ready
	serverReady := make(chan struct{})

	// Start server in goroutine
	go runServer(serverReady)

	// Wait for server to be ready
	<-serverReady
	log.Println("Server ready, starting client...")

	// Run client in main goroutine
	runClient()

	log.Println("✅ Test complete!")
}

func runServer(ready chan struct{}) {
	log.Println("[server] Starting WireGuard server (192.168.4.2)...")

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	err = dev.IpcSet(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
listen_port=51822
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.1/32
persistent_keepalive_interval=25
`)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("[server] WireGuard interface up. Starting HTTP server on :80...")

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		log.Panicln(err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("[server] > %s - %s", request.RemoteAddr, request.URL.String())
		io.WriteString(writer, "pong from combined userspace WireGuard!")
	})

	log.Println("[server] Ready. Listening on 192.168.4.2:80")

	// Signal that server is ready
	close(ready)

	// Start serving (this will block)
	srv := &http.Server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(10 * time.Second)
		cancel()
		listener.Close()
	}()

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	err = srv.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		log.Printf("[server] Error: %v", err)
	}
}

func runClient() {
	log.Println("[client] Starting WireGuard client (192.168.4.1)...")

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.1")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	err = dev.IpcSet(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
endpoint=127.0.0.1:51822
`)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("[client] WireGuard interface up. Connecting to server...")

	// Give WireGuard handshake time to complete
	time.Sleep(2 * time.Second)

	client := http.Client{
		Transport: &http.Transport{
			DialContext: tnet.DialContext,
		},
	}

	resp, err := client.Get("http://192.168.4.2/")
	if err != nil {
		log.Panic(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("[client] ✅ Response from server: %s", string(body))
}
