package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// randomAttempts bounds how often ReserveRandom retries on a collision.
const randomAttempts = 10

var errNoSubdomain = errors.New("could not allocate a free subdomain")

// Registry maps subdomains to live tunnels. It is safe for concurrent use.
//
// A subdomain goes through two states: Reserve marks it as taken while the
// handshake is still running (the map holds nil), and Activate attaches the
// tunnel once the yamux session is up. Lookup only returns active tunnels;
// Await also waits out a handshake that is just finishing.
type Registry struct {
	mu      sync.RWMutex
	tunnels map[string]*tunnel
	// pending holds a channel per reservation, closed when it is activated
	// or released.
	pending map[string]chan struct{}
}

func NewRegistry() *Registry {
	return &Registry{tunnels: make(map[string]*tunnel), pending: make(map[string]chan struct{})}
}

// Reserve claims sub. It returns false if sub is already reserved or active.
func (r *Registry) Reserve(sub string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, taken := r.tunnels[sub]; taken {
		return false
	}
	r.tunnels[sub] = nil
	r.pending[sub] = make(chan struct{})
	return true
}

// ReserveRandom claims a random 4-character hex subdomain.
func (r *Registry) ReserveRandom() (string, error) {
	for range randomAttempts {
		sub := randomSubdomain()
		if !isReserved(sub) && r.Reserve(sub) {
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
	r.settle(sub)
}

// Remove deletes sub only if it still points to t. Passing nil releases a
// reservation. The check keeps a stale tunnel from removing a newer one
// that reconnected under the same subdomain.
func (r *Registry) Remove(sub string, t *tunnel) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cur, ok := r.tunnels[sub]; ok && cur == t {
		delete(r.tunnels, sub)
		r.settle(sub)
	}
}

// settle wakes everyone waiting in Await for sub. r.mu must be held.
func (r *Registry) settle(sub string) {
	if ch, ok := r.pending[sub]; ok {
		close(ch)
		delete(r.pending, sub)
	}
}

// Lookup returns the active tunnel for sub.
func (r *Registry) Lookup(sub string) (*tunnel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t := r.tunnels[sub]
	return t, t != nil
}

// Await is Lookup for a request that may arrive while sub's handshake is
// finishing: the server answers the client before it attaches the yamux
// session, so a request sent the moment the client reports ready can find
// the subdomain still reserved. Await waits up to max for that.
func (r *Registry) Await(ctx context.Context, sub string, max time.Duration) (*tunnel, bool) {
	r.mu.RLock()
	t, ch := r.tunnels[sub], r.pending[sub]
	r.mu.RUnlock()
	if t != nil || ch == nil {
		return t, t != nil
	}
	timer := time.NewTimer(max)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
	case <-ctx.Done():
	}
	return r.Lookup(sub)
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
