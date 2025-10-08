package client

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// ClientConfig holds client configuration
type ClientConfig struct {
	ListenAddr string // Local UDP address to listen on
	ServerAddr string // Remote server UDP address
}

// Client forwards packets between local WireGuard and remote server
type Client struct {
	listenConn *net.UDPConn
	serverAddr *net.UDPAddr

	mu       sync.RWMutex
	peerAddr *net.UDPAddr // Local WireGuard peer address (learned from first packet)
}

// NewClient creates a new client instance
func NewClient(cfg *ClientConfig) (*Client, error) {
	// Resolve listen address
	listenUDPAddr, err := net.ResolveUDPAddr("udp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve listen address: %w", err)
	}

	// Create listening socket
	listenConn, err := net.ListenUDP("udp", listenUDPAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on UDP: %w", err)
	}

	// Resolve server address
	serverAddr, err := net.ResolveUDPAddr("udp", cfg.ServerAddr)
	if err != nil {
		_ = listenConn.Close()
		return nil, fmt.Errorf("failed to resolve server address: %w", err)
	}

	return &Client{
		listenConn: listenConn,
		serverAddr: serverAddr,
	}, nil
}

// Run starts the client and blocks until context is cancelled
func (c *Client) Run(ctx context.Context) error {
	buf := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, sourceAddr, err := c.listenConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("failed to read UDP packet: %w", err)
			}
		}

		packet := make([]byte, n)
		copy(packet, buf[:n])
		go c.handlePacket(packet, sourceAddr)
	}
}

// handlePacket routes packet based on source address
func (c *Client) handlePacket(packet []byte, sourceAddr *net.UDPAddr) {
	// Check if packet is from server
	if sourceAddr.String() == c.serverAddr.String() {
		// Packet from server → forward to local peer
		c.forwardToPeer(packet)
	} else {
		// Packet from local peer → learn address and forward to server
		c.learnPeerAddr(sourceAddr)
		c.forwardToServer(packet)
	}
}

// learnPeerAddr stores the local peer address
func (c *Client) learnPeerAddr(addr *net.UDPAddr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.peerAddr == nil {
		c.peerAddr = addr
	}
}

// forwardToServer sends packet to remote server
func (c *Client) forwardToServer(packet []byte) {
	_, _ = c.listenConn.WriteToUDP(packet, c.serverAddr)
}

// forwardToPeer sends packet to local WireGuard peer
func (c *Client) forwardToPeer(packet []byte) {
	c.mu.RLock()
	peerAddr := c.peerAddr
	c.mu.RUnlock()

	if peerAddr != nil {
		_, _ = c.listenConn.WriteToUDP(packet, peerAddr)
	}
}

// Close cleanly shuts down the client
func (c *Client) Close() error {
	return c.listenConn.Close()
}
