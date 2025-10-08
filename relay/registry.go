package relay

import (
	"sync"
)

// Registry maintains the mapping between peer indices and their current endpoints.
// Thread-safe for concurrent access from multiple goroutines.
type Registry struct {
	mu    sync.RWMutex
	peers map[uint32]*Endpoint
}

// NewRegistry creates a new empty peer registry
func NewRegistry() *Registry {
	return &Registry{
		peers: make(map[uint32]*Endpoint),
	}
}

// Register associates a peer index with an endpoint.
// If the index already exists, it updates the endpoint.
func (r *Registry) Register(index uint32, endpoint *Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[index] = endpoint
}

// Lookup retrieves the endpoint for a given peer index.
// Returns nil if the index is not registered.
func (r *Registry) Lookup(index uint32) *Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.peers[index]
}

// Remove deletes a peer from the registry.
// Safe to call even if the index doesn't exist.
func (r *Registry) Remove(index uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, index)
}

// Count returns the number of registered peers
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.peers)
}

// GetAllExcept returns all registered endpoints except the given source endpoint.
// Used for broadcasting handshake initiation packets to all peers except sender.
func (r *Registry) GetAllExcept(source *Endpoint) []*Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Endpoint, 0, len(r.peers))
	for _, endpoint := range r.peers {
		// Skip if this is the source endpoint (compare addresses)
		if !endpointsEqual(endpoint, source) {
			result = append(result, endpoint)
		}
	}
	return result
}

// endpointsEqual checks if two endpoints refer to the same address
func endpointsEqual(a, b *Endpoint) bool {
	if a.Type != b.Type {
		return false
	}

	if a.Type == EndpointUDP && b.Type == EndpointUDP {
		return a.UDPAddr != nil && b.UDPAddr != nil &&
		       a.UDPAddr.String() == b.UDPAddr.String()
	}

	// For stream endpoints, compare remote addresses if available
	if a.Type == EndpointStream && b.Type == EndpointStream {
		return a.StreamRemote == b.StreamRemote
	}

	return false
}
