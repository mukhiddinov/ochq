package web

import (
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *fakeClock               { return &fakeClock{t: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)} }

func TestLimiterPerKey(t *testing.T) {
	c := newClock()
	l := NewLimiter(3, 10*time.Minute, 100, time.Hour, c.now)
	for i := range 3 {
		if !l.Allow("a") {
			t.Fatalf("call %d denied", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("4th call within the window allowed")
	}
	if !l.Allow("b") {
		t.Fatal("other key denied")
	}
	c.add(10*time.Minute + time.Second)
	if !l.Allow("a") {
		t.Fatal("call after the window denied")
	}
}

func TestLimiterGlobal(t *testing.T) {
	l := NewLimiter(3, 10*time.Minute, 2, time.Hour, newClock().now)
	l.Allow("a")
	l.Allow("b")
	if l.Allow("c") {
		t.Fatal("global cap not enforced")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct{ remote, xff, want string }{
		{"172.30.0.2:5000", "1.1.1.1, 9.9.9.9", "9.9.9.9"},
		{"5.5.5.5:1", "9.9.9.9", "5.5.5.5"},
		{"172.30.0.2:5000", "", "172.30.0.2"},
	}
	for _, tt := range tests {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remote
		if tt.xff != "" {
			r.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := ClientIP(r, "172.30.0.2"); got != tt.want {
			t.Errorf("ClientIP(%s, %q) = %q, want %q", tt.remote, tt.xff, got, tt.want)
		}
	}
}
