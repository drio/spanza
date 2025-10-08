package server

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/drio/spanza/relay"
)

// UDPListener handles incoming UDP packets
type UDPListener struct {
	conn      *net.UDPConn
	processor *relay.Processor
}

// NewUDPListener creates a UDP listener bound to the given address
func NewUDPListener(addr string, processor *relay.Processor) (*UDPListener, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}

	return &UDPListener{
		conn:      conn,
		processor: processor,
	}, nil
}

// Run starts the UDP listener loop, reading and processing packets
// until the context is cancelled
func (l *UDPListener) Run(ctx context.Context) error {
	// Close connection when context is cancelled to unblock ReadFromUDP
	go func() {
		<-ctx.Done()
		l.conn.Close()
	}()

	buf := make([]byte, 2048) // Buffer for UDP packets

	for {
		n, addr, err := l.conn.ReadFromUDP(buf)
		if err != nil {
			// Check if we're shutting down (context cancelled, connection closed)
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("failed to read UDP packet: %w", err)
			}
		}

		// Process packet in a goroutine to avoid blocking the read loop
		packet := make([]byte, n)
		copy(packet, buf[:n])
		go l.handlePacket(packet, addr)
	}
}

// handlePacket processes a single packet from a source address
func (l *UDPListener) handlePacket(packet []byte, sourceAddr *net.UDPAddr) {
	// Create endpoint for the source
	source := relay.NewUDPEndpoint(sourceAddr)

	// Process the packet through the relay processor
	destinations, err := l.processor.ProcessPacket(packet, source)
	if err != nil {
		// Invalid packet, ignore
		log.Printf("[relay] Invalid packet from %s: %v", sourceAddr, err)
		return
	}

	// Forward to all destinations
	if len(destinations) > 0 {
		if len(destinations) == 1 {
			log.Printf("[relay] Forwarding packet from %s to %s", sourceAddr, destinations[0].UDPAddr)
		} else {
			log.Printf("[relay] Broadcasting packet from %s to %d peers", sourceAddr, len(destinations))
		}
		for _, dest := range destinations {
			l.forward(packet, dest)
		}
	} else {
		log.Printf("[relay] No destination for packet from %s (learning phase)", sourceAddr)
	}
}

// forward sends a packet to the destination endpoint
func (l *UDPListener) forward(packet []byte, dest *relay.Endpoint) {
	switch dest.Type {
	case relay.EndpointUDP:
		if dest.UDPAddr != nil {
			_, _ = l.conn.WriteToUDP(packet, dest.UDPAddr)
		}
	case relay.EndpointStream:
		if dest.StreamConn != nil {
			// TODO: Send via HTTPS stream
			// Will implement when we add HTTPS stream support
			_ = dest.StreamConn // noop
		}
	}
}

// Close closes the UDP connection
func (l *UDPListener) Close() error {
	return l.conn.Close()
}
