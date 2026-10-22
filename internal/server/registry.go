package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

// randomAttempts bounds how often ReserveRandom retries on a collision.
const randomAttempts = 10

var errNoSubdomain = errors.New("could not allocate a free subdomain")

// Registry maps subdomains to live tunnels. It is safe for concurrent use.
//
// A subdomain goes through two states: Reserve marks it as taken while the
// handshake is still running (the map holds nil), and Activate attaches the
// tunnel once the yamux session is up. Lookup only returns active tunnels.
type Registry struct {
	mu      sync.RWMutex
	tunnels map[string]*tunnel
}

func NewRegistry() *Registry {
	return &Registry{tunnels: make(map[string]*tunnel)}
}

// Reserve claims sub. It returns false if sub is already reserved or active.
func (r *Registry) Reserve(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.tunnels[sub]; taken {
		return false
	}
	r.tunnels[sub] = nil
	return true
}

// ReserveRandom claims a random 4-character hex subdomain.
func (r *Registry) ReserveRandom() (string, error) {
	for range randomAttempts {
		sub := randomSubdomain()
		if r.Reserve(sub) {
			return sub, nil
		}
	}
	return "", errNoSubdomain
}

// Activate attaches t to a previously reserved sub.
func (r *Registry) Activate(sub string, t *tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tunnels[sub] = t
}

// Remove deletes sub only if it still points to t. Passing nil releases a
// reservation. The check keeps a stale tunnel from removing a newer one
// that reconnected under the same subdomain.
func (r *Registry) Remove(sub string, t *tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.tunnels[sub]; ok && cur == t {
		delete(r.tunnels, sub)
	}
}

// Lookup returns the active tunnel for sub.
func (r *Registry) Lookup(sub string) (*tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tunnels[sub]
	return t, t != nil
}

// CloseAll closes every active tunnel. Their handlers remove them from the
// registry as their sessions shut down.
func (r *Registry) CloseAll() {
	r.mu.RLock()
	active := make([]*tunnel, 0, len(r.tunnels))
	for _, t := range r.tunnels {
		if t != nil {
			active = append(active, t)
		}
	}
	r.mu.RUnlock()

	// Close outside the lock: closing a session can block briefly.
	for _, t := range active {
		t.close()
	}
}

func randomSubdomain() string {
	var b [2]byte
	rand.Read(b[:]) // never fails since Go 1.24
	return hex.EncodeToString(b[:])
}
