package relay

import (
	"fmt"

	"github.com/drio/spanza/packet"
)

// Processor handles packet processing logic, learning peer endpoints
// and determining forwarding destinations.
type Processor struct {
	registry *Registry
}

// NewProcessor creates a processor with the given registry
func NewProcessor(registry *Registry) *Processor {
	return &Processor{
		registry: registry,
	}
}

// ProcessPacket processes an incoming WireGuard packet from a source endpoint.
// It updates the registry with learned sender information and returns the
// destination endpoints where the packet should be forwarded.
//
// For handshake initiation packets (no receiver index), it broadcasts to all
// known peers except the sender. For all other packets (with receiver index),
// it returns a single destination.
//
// Returns empty slice if no destinations are available.
func (p *Processor) ProcessPacket(data []byte, source *Endpoint) ([]*Endpoint, error) {
	msg, err := packet.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse packet: %w", err)
	}

	// Learn sender's endpoint if this packet has a sender index
	if msg.Sender != nil {
		p.registry.Register(*msg.Sender, source)
	}

	// Determine where to forward based on receiver index
	if msg.Receiver != nil {
		// Packet has receiver index - forward to specific peer
		dest := p.registry.Lookup(*msg.Receiver)
		if dest != nil {
			return []*Endpoint{dest}, nil
		}
		return []*Endpoint{}, nil
	}

	// No receiver index means this is a handshake initiation packet.
	// Broadcast to all known peers except the sender.
	destinations := p.registry.GetAllExcept(source)
	return destinations, nil
}
