package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Limiter allows perKey events per key within perKeyWindow, and global
// events in total within globalWindow (a fuse against floods from many
// addresses). Only allowed events count.
type Limiter struct {
	mu                         sync.Mutex
	perKey, global             int
	perKeyWindow, globalWindow time.Duration
	keys                       map[string][]time.Time
	all                        []time.Time
	now                        func() time.Time
}

func NewLimiter(perKey int, perKeyWindow time.Duration, global int, globalWindow time.Duration, now func() time.Time) *Limiter {
	return &Limiter{perKey: perKey, perKeyWindow: perKeyWindow, global: global, globalWindow: globalWindow, keys: map[string][]time.Time{}, now: now}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	// Forget every key's old events, so the map cannot grow without bound.
	for k, ts := range l.keys {
		if ts = since(ts, now.Add(-l.perKeyWindow)); len(ts) == 0 {
			delete(l.keys, k)
		} else {
			l.keys[k] = ts
		}
	}
	l.all = since(l.all, now.Add(-l.globalWindow))
	if len(l.keys[key]) >= l.perKey || len(l.all) >= l.global {
		return false
	}
	l.keys[key] = append(l.keys[key], now)
	l.all = append(l.all, now)
	return true
}

// since drops the times not after cutoff from the sorted ts.
func since(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && !ts[i].After(cutoff) {
		i++
	}
	return ts[i:]
}

// ClientIP returns the visitor's address. X-Forwarded-For is trusted only
// from trustedProxy (Traefik), whose own entry is the rightmost one; anyone
// else could write whatever they like into it.
func ClientIP(r *http.Request, trustedProxy string) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host != trustedProxy {
		return host
	}
	xff := r.Header.Values("X-Forwarded-For")
	if len(xff) == 0 {
		return host
	}
	parts := strings.Split(xff[len(xff)-1], ",")
	if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
		return ip
	}
	return host
}
