package relay

import (
	"net"
	"testing"
)

func TestRegistryBasicOperations(t *testing.T) {
	registry := NewRegistry()

	// Initially empty
	if count := registry.Count(); count != 0 {
		t.Errorf("new registry should be empty, got %d peers", count)
	}

	// Register a UDP peer
	udpAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	endpoint1 := NewUDPEndpoint(udpAddr)
	registry.Register(12345, endpoint1)

	if count := registry.Count(); count != 1 {
		t.Errorf("expected 1 peer after registration, got %d", count)
	}

	// Lookup the registered peer
	result := registry.Lookup(12345)
	if result == nil {
		t.Fatal("expected to find peer 12345, got nil")
	}
	if !result.Equal(endpoint1) {
		t.Errorf("endpoint mismatch: expected %v, got %v", endpoint1, result)
	}

	// Lookup non-existent peer
	if result := registry.Lookup(99999); result != nil {
		t.Errorf("expected nil for non-existent peer, got %v", result)
	}
}

func TestRegistryUpdate(t *testing.T) {
	registry := NewRegistry()

	// Register initial endpoint
	udpAddr1 := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	endpoint1 := NewUDPEndpoint(udpAddr1)
	registry.Register(12345, endpoint1)

	// Update with different endpoint
	udpAddr2 := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 51821}
	endpoint2 := NewUDPEndpoint(udpAddr2)
	registry.Register(12345, endpoint2)

	// Should still have only 1 peer
	if count := registry.Count(); count != 1 {
		t.Errorf("expected 1 peer after update, got %d", count)
	}

	// Should return the updated endpoint
	result := registry.Lookup(12345)
	if !result.Equal(endpoint2) {
		t.Errorf("expected updated endpoint %v, got %v", endpoint2, result)
	}
}

func TestRegistryRemove(t *testing.T) {
	registry := NewRegistry()

	// Register a peer
	udpAddr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 51820}
	endpoint := NewUDPEndpoint(udpAddr)
	registry.Register(12345, endpoint)

	// Remove the peer
	registry.Remove(12345)

	if count := registry.Count(); count != 0 {
		t.Errorf("expected 0 peers after removal, got %d", count)
	}

	if result := registry.Lookup(12345); result != nil {
		t.Errorf("expected nil after removal, got %v", result)
	}

	// Removing non-existent peer should not panic
	registry.Remove(99999)
}

func TestRegistryMultiplePeers(t *testing.T) {
	registry := NewRegistry()

	// Register multiple peers
	for i := uint32(1); i <= 5; i++ {
		addr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, byte(i)), Port: int(51820 + i)}
		endpoint := NewUDPEndpoint(addr)
		registry.Register(i*1000, endpoint)
	}

	if count := registry.Count(); count != 5 {
		t.Errorf("expected 5 peers, got %d", count)
	}

	// Verify each peer can be looked up
	for i := uint32(1); i <= 5; i++ {
		result := registry.Lookup(i * 1000)
		if result == nil {
			t.Errorf("expected to find peer %d, got nil", i*1000)
		}
	}
}
