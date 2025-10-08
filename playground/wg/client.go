package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/netip"
	"os"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

func main() {
	log.Println("Starting WireGuard client (192.168.4.1)...")

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.MustParseAddr("192.168.4.1")},
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

	config := fmt.Sprintf(`private_key=087ec6e14bbed210e7215cdc73468dfa23f080a1bfb8665b2fd809bd99d28379
public_key=c4c8e984c5322c8184c72265b92b250fdb63688705f504ba003c88f03393cf28
allowed_ip=0.0.0.0/0
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

	log.Println("WireGuard interface up. Connecting to server...")

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

	log.Printf("✅ Response from server: %s", string(body))
}
