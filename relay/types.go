package relay

import (
	"fmt"
	"net"
	"time"

	"github.com/coder/websocket"
)

// EndpointType represents the type of network endpoint
type EndpointType int

const (
	EndpointUDP EndpointType = iota
	EndpointWebSocket
)

func (et EndpointType) String() string {
	switch et {
	case EndpointUDP:
		return "UDP"
	case EndpointWebSocket:
		return "WebSocket"
	default:
		return "Unknown"
	}
}

// Endpoint represents a peer's network location
type Endpoint struct {
	Type EndpointType
	// For UDP endpoints
	UDPAddr *net.UDPAddr
	// For WebSocket endpoints
	WSConn   *websocket.Conn
	WSRemote string // Remote address string for WebSocket
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

// NewWebSocketEndpoint creates an endpoint for a WebSocket connection
func NewWebSocketEndpoint(conn *websocket.Conn, remoteAddr string) *Endpoint {
	return &Endpoint{
		Type:     EndpointWebSocket,
		WSConn:   conn,
		WSRemote: remoteAddr,
		LastSeen: time.Now(),
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
	case EndpointWebSocket:
		if e.WSRemote != "" {
			return fmt.Sprintf("WS:%s", e.WSRemote)
		}
		return "WS:<nil>"
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
	case EndpointWebSocket:
		return e.WSRemote == other.WSRemote
	default:
		return false
	}
}

// Peer represents a WireGuard peer with its current endpoint
type Peer struct {
	Index    uint32
	Endpoint *Endpoint
}
