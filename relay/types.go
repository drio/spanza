package relay

import (
	"fmt"
	"io"
	"net"
	"time"
)

// EndpointType represents the type of network endpoint
type EndpointType int

const (
	EndpointUDP EndpointType = iota
	EndpointStream
)

func (et EndpointType) String() string {
	switch et {
	case EndpointUDP:
		return "UDP"
	case EndpointStream:
		return "Stream"
	default:
		return "Unknown"
	}
}

// Endpoint represents a peer's network location
type Endpoint struct {
	Type EndpointType
	// For UDP endpoints
	UDPAddr *net.UDPAddr
	// For HTTPS stream endpoints (HTTP Upgrade)
	StreamConn   io.ReadWriteCloser
	StreamRemote string // Remote address string for stream
	// Last time this endpoint was seen
	LastSeen time.Time
}

// NewUDPEndpoint creates an endpoint for a UDP address
func NewUDPEndpoint(addr *net.UDPAddr) *Endpoint {
	return &Endpoint{
		Type:     EndpointUDP,
		UDPAddr:  addr,
		LastSeen: time.Now(),
	}
}

// NewStreamEndpoint creates an endpoint for an HTTPS stream connection
func NewStreamEndpoint(conn io.ReadWriteCloser, remoteAddr string) *Endpoint {
	return &Endpoint{
		Type:         EndpointStream,
		StreamConn:   conn,
		StreamRemote: remoteAddr,
		LastSeen:     time.Now(),
	}
}

// String returns a string representation of the endpoint
func (e *Endpoint) String() string {
	switch e.Type {
	case EndpointUDP:
		if e.UDPAddr != nil {
			return fmt.Sprintf("UDP:%s", e.UDPAddr.String())
		}
		return "UDP:<nil>"
	case EndpointStream:
		if e.StreamRemote != "" {
			return fmt.Sprintf("Stream:%s", e.StreamRemote)
		}
		return "Stream:<nil>"
	default:
		return "Unknown"
	}
}

// Equal checks if two endpoints are the same
func (e *Endpoint) Equal(other *Endpoint) bool {
	if e == nil || other == nil {
		return e == other
	}
	if e.Type != other.Type {
		return false
	}
	switch e.Type {
	case EndpointUDP:
		return e.UDPAddr != nil && other.UDPAddr != nil &&
			e.UDPAddr.IP.Equal(other.UDPAddr.IP) &&
			e.UDPAddr.Port == other.UDPAddr.Port
	case EndpointStream:
		return e.StreamRemote == other.StreamRemote
	default:
		return false
	}
}

// Peer represents a WireGuard peer with its current endpoint
type Peer struct {
	Index    uint32
	Endpoint *Endpoint
}
