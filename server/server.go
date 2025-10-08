package server

import (
	"context"
	"fmt"

	"github.com/drio/spanza/relay"
)

// ServerConfig holds server configuration and dependencies
type ServerConfig struct {
	UDPAddr   string
	Registry  *relay.Registry
	Processor *relay.Processor
}

// Server manages UDP listener and packet relaying
type Server struct {
	udpListener *UDPListener
	registry    *relay.Registry
	processor   *relay.Processor
}

// NewServer creates a new server instance with the provided configuration
func NewServer(cfg *ServerConfig) (*Server, error) {
	udpListener, err := NewUDPListener(cfg.UDPAddr, cfg.Processor)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP listener: %w", err)
	}

	return &Server{
		udpListener: udpListener,
		registry:    cfg.Registry,
		processor:   cfg.Processor,
	}, nil
}

// Run starts the server and blocks until context is cancelled
func (s *Server) Run(ctx context.Context) error {
	return s.udpListener.Run(ctx)
}

// Close cleanly shuts down the server
func (s *Server) Close() error {
	return s.udpListener.Close()
}
