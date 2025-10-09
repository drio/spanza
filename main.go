package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

const version = "0.2.0-derp"

var (
	derpURL    = flag.String("derp-url", "https://derp.tailscale.com/derp", "DERP server URL")
	// DERP key is separate from WireGuard key - used only for DERP identity/addressing.
	// Could use WG key instead (like Tailscale does), but keeping separate for cleaner separation.
	keyFile    = flag.String("key-file", "", "Path to private key file (will generate if missing)")
	remotePeer = flag.String("remote-peer", "", "Remote peer's DERP public key (nodekey:...)")
	// TODO: could be auto-discovered from first UDP packet instead of manual config
	wgEndpoint = flag.String("wg-endpoint", "127.0.0.1:51820", "Local WireGuard endpoint (IP:port)")
	listenAddr = flag.String("listen", ":51821", "UDP listen address for WireGuard")
	verbose    = flag.Bool("verbose", false, "Enable verbose logging")
	showVersion = flag.Bool("version", false, "Show version and exit")
	showPubkey = flag.Bool("show-pubkey", false, "Show DERP public key and exit")
)

// Gateway handles UDP <-> DERP translation
type Gateway struct {
	derpClient    *derphttp.Client
	privateKey    key.NodePrivate
	udpConn       *net.UDPConn
	remotePeerKey key.NodePublic
	wgAddr        *net.UDPAddr
	ctx           context.Context
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("spanza %s - WireGuard to DERP gateway\n", version)
		return
	}

	if *showPubkey {
		privKey, err := loadOrGenerateKey(*keyFile)
		if err != nil {
			log.Fatalf("Failed to load/generate key: %v", err)
		}
		fmt.Printf("%s\n", privKey.Public())
		return
	}

	if *remotePeer == "" {
		log.Fatal("--remote-peer is required")
	}

	var remotePeerKey key.NodePublic
	if err := remotePeerKey.UnmarshalText([]byte(*remotePeer)); err != nil {
		log.Fatalf("Invalid remote peer key: %v", err)
	}

	privKey, err := loadOrGenerateKey(*keyFile)
	if err != nil {
		log.Fatalf("Failed to load/generate key: %v", err)
	}

	if *verbose {
		log.Printf("Our public key: %s", privKey.Public())
		log.Printf("Remote peer key: %s", remotePeerKey)
	}

	wgAddr, err := net.ResolveUDPAddr("udp", *wgEndpoint)
	if err != nil {
		log.Fatalf("Invalid WireGuard endpoint: %v", err)
	}

	listenUDPAddr, err := net.ResolveUDPAddr("udp", *listenAddr)
	if err != nil {
		log.Fatalf("Invalid listen address: %v", err)
	}

	udpConn, err := net.ListenUDP("udp", listenUDPAddr)
	if err != nil {
		log.Fatalf("Failed to listen on UDP: %v", err)
	}
	defer udpConn.Close()

	log.Printf("UDP listener started on %s", *listenAddr)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gw := &Gateway{
		privateKey:    privKey,
		udpConn:       udpConn,
		remotePeerKey: remotePeerKey,
		wgAddr:        wgAddr,
		ctx:           ctx,
	}

	if err := gw.connectDERP(); err != nil {
		log.Fatalf("Failed to connect to DERP: %v", err)
	}
	defer gw.derpClient.Close()

	log.Printf("Connected to DERP server: %s", *derpURL)
	log.Printf("Gateway running. Press Ctrl+C to stop.")

	errCh := make(chan error, 2)
	go func() { errCh <- gw.udpToDERP() }()
	go func() { errCh <- gw.derpToUDP() }()

	select {
	case err := <-errCh:
		if err != nil {
			log.Printf("Gateway error: %v", err)
		}
	case <-ctx.Done():
		log.Printf("Shutting down...")
	}
}

func (gw *Gateway) connectDERP() error {
	logf := func(format string, args ...any) {
		if *verbose {
			log.Printf("[DERP] "+format, args...)
		}
	}

	// netmon (network monitor) tracks network state changes (interface up/down, IP changes, etc).
	// Use static netmon (doesn't monitor actual network changes) - fine for basic relay.
	// TODO: Consider using real netmon for production with automatic reconnection on network changes.
	netMon := netmon.NewStatic()

	client, err := derphttp.NewClient(gw.privateKey, *derpURL, logf, netMon)
	if err != nil {
		return fmt.Errorf("failed to create DERP client: %w", err)
	}

	gw.derpClient = client
	return nil
}

func (gw *Gateway) udpToDERP() error {
	buf := make([]byte, 65535)

	for {
		select {
		case <-gw.ctx.Done():
			return nil
		default:
		}

		n, addr, err := gw.udpConn.ReadFromUDP(buf)
		if err != nil {
			if gw.ctx.Err() != nil {
				return nil
			}
			log.Printf("UDP read error: %v", err)
			continue
		}

		if *verbose {
			log.Printf("UDP recv: %d bytes from %s", n, addr)
		}

		if err := gw.derpClient.Send(gw.remotePeerKey, buf[:n]); err != nil {
			log.Printf("DERP send error: %v", err)
			continue
		}

		if *verbose {
			log.Printf("DERP sent: %d bytes to %s", n, gw.remotePeerKey.ShortString())
		}
	}
}

func (gw *Gateway) derpToUDP() error {
	for {
		select {
		case <-gw.ctx.Done():
			return nil
		default:
		}

		msg, err := gw.derpClient.Recv()
		if err != nil {
			if gw.ctx.Err() != nil {
				return nil
			}
			log.Printf("DERP recv error: %v", err)
			continue
		}

		switch m := msg.(type) {
		case derp.ReceivedPacket:
			if *verbose {
				log.Printf("DERP recv: %d bytes from %s", len(m.Data), m.Source.ShortString())
			}

			n, err := gw.udpConn.WriteToUDP(m.Data, gw.wgAddr)
			if err != nil {
				log.Printf("UDP write error: %v", err)
				continue
			}

			if *verbose {
				log.Printf("UDP sent: %d bytes to %s", n, gw.wgAddr)
			}

		default:
			if *verbose {
				log.Printf("DERP: received non-packet message: %T", msg)
			}
		}
	}
}

func loadOrGenerateKey(path string) (key.NodePrivate, error) {
	if path == "" {
		// Ephemeral key - fine since DERP key is just for addressing, not encryption.
		// Remote peer will need to know the new public key each run.
		return key.NewNode(), nil
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var privKey key.NodePrivate
		if err := privKey.UnmarshalText(bytes.TrimSpace(data)); err != nil {
			return key.NodePrivate{}, fmt.Errorf("failed to parse key: %w", err)
		}
		return privKey, nil
	}

	privKey := key.NewNode()
	marshaled, err := privKey.MarshalText()
	if err != nil {
		return key.NodePrivate{}, fmt.Errorf("failed to marshal key: %w", err)
	}
	// MarshalText returns the key with "nodekey:" prefix, save it as-is
	if err := os.WriteFile(path, marshaled, 0600); err != nil {
		return key.NodePrivate{}, fmt.Errorf("failed to save key: %w", err)
	}

	log.Printf("Generated new key and saved to %s", path)
	return privKey, nil
}
