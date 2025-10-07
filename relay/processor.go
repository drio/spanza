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
// destination endpoint where the packet should be forwarded.
// Returns nil destination if the receiver is unknown.
func (p *Processor) ProcessPacket(data []byte, source *Endpoint) (*Endpoint, error) {
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
		dest := p.registry.Lookup(*msg.Receiver)
		return dest, nil
	}

	// No receiver index means this is an initiation packet
	// (first message of handshake), nothing to forward yet
	return nil, nil
}
