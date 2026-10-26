package server

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/mukhiddinov/ochq/internal/proto"
)

func TestRegistryReserve(t *testing.T) {
	r := NewRegistry()
	if !r.Reserve("demo") {
		t.Fatal("first Reserve should succeed")
	}
	if r.Reserve("demo") {
		t.Fatal("second Reserve should fail")
	}
	if _, ok := r.Lookup("demo"); ok {
		t.Fatal("reserved but inactive subdomain must not be routable")
	}

	tun := &tunnel{subdomain: "demo"}
	r.Activate("demo", tun)
	if got, ok := r.Lookup("demo"); !ok || got != tun {
		t.Fatal("Lookup should return the active tunnel")
	}

	// A stale tunnel must not remove the current one.
	r.Remove("demo", &tunnel{subdomain: "demo"})
	if _, ok := r.Lookup("demo"); !ok {
		t.Fatal("Remove with a different tunnel must be a no-op")
	}
	r.Remove("demo", tun)
	if !r.Reserve("demo") {
		t.Fatal("subdomain should be free after Remove")
	}
}

// A request that races the end of a handshake must reach the tunnel instead
// of getting 404: Await waits for the reservation to become active.
func TestRegistryAwait(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()

	start := time.Now()
	if _, ok := r.Await(ctx, "nope", time.Second); ok || time.Since(start) > 100*time.Millisecond {
		t.Fatal("an unknown subdomain must fail at once, without waiting")
	}

	r.Reserve("demo")
	tun := &tunnel{subdomain: "demo"}
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Activate("demo", tun)
	}()
	if got, ok := r.Await(ctx, "demo", 5*time.Second); !ok || got != tun {
		t.Fatal("Await should return the tunnel once it is activated")
	}

	// A handshake that fails releases the reservation and wakes waiters.
	r.Reserve("gone")
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Remove("gone", nil)
	}()
	start = time.Now()
	if _, ok := r.Await(ctx, "gone", 5*time.Second); ok || time.Since(start) > 2*time.Second {
		t.Fatal("a released reservation must end the wait without a tunnel")
	}

	// A reservation that never finishes gives up after max.
	r.Reserve("stuck")
	if _, ok := r.Await(ctx, "stuck", 30*time.Millisecond); ok {
		t.Fatal("Await must time out on a reservation that never activates")
	}
}

func TestRegistryReserveRandom(t *testing.T) {
	r := NewRegistry()
	seen := map[string]bool{}
	for range 100 {
		sub, err := r.ReserveRandom()
		if err != nil {
			t.Fatal(err)
		}
		if len(sub) != 4 || seen[sub] {
			t.Fatalf("bad or duplicate subdomain %q", sub)
		}
		seen[sub] = true
	}
}

func TestSubdomainFromHost(t *testing.T) {
	s := New(Config{Domain: "Ochq.uz"})
	tests := map[string]string{
		"x8k2.ochq.uz":       "x8k2",
		"X8K2.OCHQ.UZ:8080":  "x8k2",
		"my-api.ochq.uz.":    "my-api",
		"ochq.uz":            "",
		"a.b.ochq.uz":        "",
		"x8k2.evil.com":      "",
		"x8k2.notochq.uz":    "",
		"[::1]:8080":         "",
		"x8k2.ochq.uz.evil.": "",
	}
	for host, want := range tests {
		got, ok := s.subdomainFromHost(host)
		if got != want || ok != (want != "") {
			t.Errorf("subdomainFromHost(%q) = %q, %v; want %q", host, got, ok, want)
		}
	}
}

func TestHandshakeRejects(t *testing.T) {
	tests := []struct {
		name  string
		hello proto.Hello
		code  proto.ErrorCode
	}{
		{"version", proto.Hello{Version: 99, Type: proto.TunnelHTTP}, proto.ErrCodeVersionMismatch},
		{"type", proto.Hello{Version: proto.Version, Type: "udp"}, proto.ErrCodeBadRequest},
		{"token", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "wrong"}, proto.ErrCodeUnauthorized},
		{"invalid", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: "a_b"}, proto.ErrCodeInvalidSubdomain},
		{"taken", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: "myapp"}, proto.ErrCodeSubdomainTaken},
		{"reserved", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: "www"}, proto.ErrCodeInvalidSubdomain},
		{"reserved case", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: " Demo "}, proto.ErrCodeInvalidSubdomain},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(Config{Domain: "ochq.uz", Token: "s3cret"})
			s.registry.Reserve("myapp")

			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			errc := make(chan error, 1)
			go func() {
				_, err := s.handshake(serverConn)
				serverConn.Close()
				errc <- err
			}()

			if err := proto.WriteHello(clientConn, &tt.hello); err != nil {
				t.Fatal(err)
			}
			resp, err := proto.ReadHelloResponse(clientConn)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Code != tt.code {
				t.Fatalf("code = %q, want %q (%s)", resp.Code, tt.code, resp.Error)
			}

			var he *proto.HandshakeError
			if err := <-errc; !errors.As(err, &he) || he.Code != tt.code {
				t.Fatalf("server error = %v, want code %q", err, tt.code)
			}
		})
	}
}

func TestIsReserved(t *testing.T) {
	for _, s := range []string{"www", "api", "admin", "mail", "login", "status", "docs", "blog", "app", "dashboard", "ochq", "tunnel", "pkg", "demo"} {
		if !isReserved(s) {
			t.Errorf("%q should be reserved", s)
		}
	}
	for _, s := range []string{"myapp", "a7f3", "demo2", "wwww"} {
		if isReserved(s) {
			t.Errorf("%q should not be reserved", s)
		}
	}
}

func TestPublicURL(t *testing.T) {
	if got := New(Config{Domain: "ochq.uz", PublicScheme: "https"}).publicURL("x8k2"); got != "https://x8k2.ochq.uz" {
		t.Errorf("got %q", got)
	}
	if got := New(Config{Domain: "localhost", PublicPort: 8080}).publicURL("demo"); got != "http://demo.localhost:8080" {
		t.Errorf("got %q", got)
	}
}
