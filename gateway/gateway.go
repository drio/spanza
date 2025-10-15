package gateway

import (
	"context"
	"fmt"
	"log"
	"net"

	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

// UDPConn is an interface that both *net.UDPConn and *gonet.UDPConn satisfy.
// This allows the gateway to work with either kernel UDP or userspace UDP.
type UDPConn interface {
	ReadFrom([]byte) (int, net.Addr, error)
	WriteTo([]byte, net.Addr) (int, error)
	Close() error
}

// Config holds the configuration for a Spanza gateway.
type Config struct {
	// Prefix is used for logging (e.g., "[gateway]", "[peer1-gw]")
	Prefix string

	// DERP configuration
	DerpURL         string // e.g., "https://derp.tailscale.com/derp"
	PrivKeyStr      string // This peer's DERP private key (e.g., "privkey:...")
	RemotePubKeyStr string // Remote peer's DERP public key (e.g., "nodekey:...")

	// WireGuard endpoint to forward received DERP packets to
	WGEndpoint string // e.g., "127.0.0.1:51820"

	// Optional: enable verbose logging
	Verbose bool
}

// Run starts a Spanza gateway that forwards packets between UDP and DERP.
//
// The gateway performs two operations concurrently:
//  1. UDP → DERP: Reads packets from udpConn, sends to remote peer via DERP
//  2. DERP → UDP: Receives packets from DERP, writes to WireGuard endpoint via udpConn
//
// The function blocks until ctx is cancelled.
func Run(ctx context.Context, cfg Config, udpConn UDPConn) error {
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "[gateway]"
	}

	log.Printf("%s Starting Spanza gateway (UDP ↔ DERP)...", prefix)

	// Parse DERP private key
	var privKey key.NodePrivate
	if err := privKey.UnmarshalText([]byte(cfg.PrivKeyStr)); err != nil {
		return fmt.Errorf("%s failed to parse private key: %w", prefix, err)
	}

	// Parse remote peer's DERP public key
	var remotePubKey key.NodePublic
	if err := remotePubKey.UnmarshalText([]byte(cfg.RemotePubKeyStr)); err != nil {
		return fmt.Errorf("%s failed to parse remote public key: %w", prefix, err)
	}

	if cfg.Verbose {
		log.Printf("%s Will send to remote DERP key: %s", prefix, remotePubKey.ShortString())
	}

	// Resolve WireGuard endpoint (where to send received DERP packets)
	wgAddr, err := net.ResolveUDPAddr("udp", cfg.WGEndpoint)
	if err != nil {
		return fmt.Errorf("%s invalid WireGuard endpoint: %w", prefix, err)
	}

	// Create DERP client
	netMon := netmon.NewStatic()
	logf := func(format string, args ...any) {
		if cfg.Verbose {
			log.Printf("[derp] "+format, args...)
		}
	}

	derpClient, err := derphttp.NewClient(privKey, cfg.DerpURL, logf, netMon)
	if err != nil {
		return fmt.Errorf("%s failed to create DERP client: %w", prefix, err)
	}
	defer derpClient.Close()

	log.Printf("%s DERP client created (connection will happen automatically)", prefix)
	log.Printf("%s Gateway ready (UDP ↔ DERP)", prefix)

	// Close connections when context is cancelled
	// This will wake up any blocked ReadFrom/Recv calls cleanly
	go func() {
		<-ctx.Done()
		udpConn.Close()
		derpClient.Close() // This will interrupt the blocking Recv() call
	}()

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

			n, _, err := udpConn.ReadFrom(buf)
			if err != nil {
				// Connection closed (context cancellation closes udpConn)
				return
			}

			if cfg.Verbose {
				log.Printf("%s → Received %d bytes in the UDP connection, sending to DERP", prefix, n)
			}

			// Send to remote peer via DERP
			if err := derpClient.Send(remotePubKey, buf[:n]); err != nil {
				log.Printf("%s DERP send error: %v", prefix, err)
			} else if cfg.Verbose {
				log.Printf("%s ✓ Sent %d bytes to remote peer via DERP", prefix, n)
			}
		}
	}()

	// Goroutine: DERP → UDP
	// Receive packets from DERP, send to WireGuard
	go func() {
		log.Printf("%s DERP receive loop started", prefix)
		for {
			select {
			case <-ctx.Done():
				log.Printf("%s DERP receive loop exiting (context done)", prefix)
				return
			default:
			}

			log.Printf("%s Waiting for DERP message...", prefix)
			msg, err := derpClient.Recv()
			if err != nil {
				if ctx.Err() != nil {
					log.Printf("%s DERP receive loop exiting (context error)", prefix)
					return
				}
				log.Printf("%s DERP recv error: %v", prefix, err)
				continue
			}

			log.Printf("%s Received DERP message type: %T", prefix, msg)
			// Only handle received packets
			switch m := msg.(type) {
			case derp.ReceivedPacket:
				if cfg.Verbose {
					log.Printf("%s ← Received %d bytes from DERP, writing to UDP connection", prefix, len(m.Data))
				}

				_, err := udpConn.WriteTo(m.Data, wgAddr)
				if err != nil {
					log.Printf("%s UDP write error: %v", prefix, err)
				} else if cfg.Verbose {
					log.Printf("%s ✓ Wrote %d bytes to UDP connection", prefix, len(m.Data))
				}
			}
		}
	}()

	<-ctx.Done()
	log.Printf("%s Gateway shutting down", prefix)
	return nil
}
