package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func main() {
	log.Println("Starting WireGuard server (192.168.4.2)...")

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.2")},
		[]netip.Addr{netip.MustParseAddr("8.8.8.8")},
		1420,
	)
	if err != nil {
		log.Panic(err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))

	// TODO: Set RELAY_ENDPOINT environment variable to your relay server's public IP
	// Example: export RELAY_ENDPOINT=192.0.2.1:51820
	relayEndpoint := "127.0.0.1:51820" // Default to localhost for testing
	if endpoint := os.Getenv("RELAY_ENDPOINT"); endpoint != "" {
		relayEndpoint = endpoint
	}

	log.Printf("Configuring to connect via relay: %s", relayEndpoint)

	config := fmt.Sprintf(`private_key=003ed5d73b55806c30de3f8a7bdab38af13539220533055e635690b8b87ad641
public_key=f928d4f6c1b86c12f2562c10b07c555c5c57fd00f59e90c8d8d88767271cbf7c
allowed_ip=192.168.4.1/32
endpoint=%s
persistent_keepalive_interval=1
`, relayEndpoint)

	err = dev.IpcSet(config)
	if err != nil {
		log.Panic(err)
	}
	err = dev.Up()
	if err != nil {
		log.Panic(err)
	}

	log.Println("WireGuard interface up. Starting HTTP server on :80...")

	listener, err := tnet.ListenTCP(&net.TCPAddr{Port: 80})
	if err != nil {
		log.Panicln(err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		log.Printf("> %s - %s - %s", request.RemoteAddr, request.URL.String(), request.UserAgent())
		io.WriteString(writer, "pong from userspace WireGuard!")
	})

	log.Println("Server ready. Listening on 192.168.4.2:80")
	err = http.Serve(listener, nil)
	if err != nil {
		log.Panicln(err)
	}
}
