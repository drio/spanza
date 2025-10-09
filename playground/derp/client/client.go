package client

import (
	"context"
	"fmt"
	"log"

	"go4.org/mem"
	"tailscale.com/derp"
	"tailscale.com/derp/derphttp"
	"tailscale.com/net/netmon"
	"tailscale.com/types/key"
)

// DERPClient wraps the derphttp.Client with convenience methods
type DERPClient struct {
	client     *derphttp.Client
	privateKey key.NodePrivate
	publicKey  key.NodePublic
}

// NewDERPClient creates a new DERP client connection
func NewDERPClient(privateKey key.NodePrivate, serverURL string) (*DERPClient, error) {
	publicKey := privateKey.Public()

	// Create network monitor (required by DERP client)
	netMon := netmon.NewStatic()

	// Create DERP client
	client, err := derphttp.NewClient(privateKey, serverURL, log.Printf, netMon)
	if err != nil {
		return nil, fmt.Errorf("create DERP client: %w", err)
	}

	// Connect to server
	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connect to DERP server: %w", err)
	}

	return &DERPClient{
		client:     client,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// PublicKey returns this client's public key
func (c *DERPClient) PublicKey() key.NodePublic {
	return c.publicKey
}

// Send sends a message to a peer
func (c *DERPClient) Send(peerKey key.NodePublic, data []byte) error {
	return c.client.Send(peerKey, data)
}

// Recv receives a message from the DERP server
func (c *DERPClient) Recv() (derp.ReceivedMessage, error) {
	return c.client.Recv()
}

// Close closes the DERP client connection
func (c *DERPClient) Close() error {
	return c.client.Close()
}

// ParsePublicKey parses a public key string (with or without nodekey: prefix)
func ParsePublicKey(keyStr string) (key.NodePublic, error) {
	// Strip the nodekey: prefix if present
	if len(keyStr) > 8 && keyStr[:8] == "nodekey:" {
		keyStr = keyStr[8:]
	}
	return key.ParseNodePublicUntyped(mem.S(keyStr))
}

// ParsePrivateKey parses a private key string (with or without privkey: prefix)
func ParsePrivateKey(keyStr string) (key.NodePrivate, error) {
	// Strip the privkey: prefix if present
	if len(keyStr) > 8 && keyStr[:8] == "privkey:" {
		keyStr = keyStr[8:]
	}
	return key.ParseNodePrivateUntyped(mem.S(keyStr))
}
