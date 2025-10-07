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
