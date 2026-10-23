// Package client implements the ochq side of a tunnel: it connects to
// ochqd and serves every incoming stream by proxying it to a local service.
package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/mukhiddinov/ochq/internal/proto"
	"github.com/mukhiddinov/ochq/internal/tlsutil"
	"github.com/mukhiddinov/ochq/pkg/version"
)

const (
	dialTimeout      = 10 * time.Second
	localDialTimeout = 5 * time.Second

	defaultMinBackoff = time.Second
	defaultMaxBackoff = 30 * time.Second
	// A session that stayed up this long resets the backoff, so a single
	// blip after hours of uptime reconnects after MinBackoff again.
	stableSession = time.Minute
)

// A yamux session hands out streams like a listener hands out connections,
// which is what lets an ordinary http.Server serve the tunnel.
var _ net.Listener = (*yamux.Session)(nil)

type Config struct {
	ServerAddr string // ochqd tunnel address, e.g. "tunnel.ochq.uz:9000"
	LocalAddr  string // local service, e.g. "127.0.0.1:3000"
	Subdomain  string // requested subdomain; empty lets the server pick
	Token      string
	// Fingerprint pins the server's self-signed key ("SHA256:...").
	// Empty means the certificate must be signed by a trusted CA.
	Fingerprint string

	// OnReady is called every time the tunnel comes up, including after
	// a reconnect.
	OnReady func(*proto.HelloResponse)
	// OnReconnect is called after a session ends, before waiting delay.
	OnReconnect func(attempt int, delay time.Duration, err error)
	// OnRequest is called when a proxied request completes.
	OnRequest func(RequestLog)

	// Bounds for the exponential reconnect backoff (default 1s and 30s).
	MinBackoff, MaxBackoff time.Duration
}

// Run keeps the tunnel up until ctx is cancelled (then it returns nil).
// When the connection drops it reconnects with exponential backoff, asking
// for the same subdomain so the public URL survives. Errors that retrying
// cannot fix — bad token, fingerprint mismatch, invalid subdomain — are
// returned immediately.
func Run(ctx context.Context, cfg Config) error {
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = defaultMinBackoff
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = defaultMaxBackoff
	}
	handler := logRequests(newLocalProxy(cfg.LocalAddr), cfg.OnRequest)

	connected := false
	for attempt := 0; ; {
		start := time.Now()
		err := runSession(ctx, cfg, handler, func(resp *proto.HelloResponse) {
			// Come back on the same URL after a reconnect.
			cfg.Subdomain = resp.Subdomain
			connected = true
			if cfg.OnReady != nil {
				cfg.OnReady(resp)
			}
		})
		if ctx.Err() != nil {
			return nil
		}
		if permanent(err, connected) {
			return err
		}

		if time.Since(start) >= stableSession {
			attempt = 0
		}
		delay := backoff(attempt, cfg.MinBackoff, cfg.MaxBackoff)
		attempt++
		if cfg.OnReconnect != nil {
			cfg.OnReconnect(attempt, delay, err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

// runSession connects once and serves the tunnel until it closes.
func runSession(ctx context.Context, cfg Config, handler http.Handler, onReady func(*proto.HelloResponse)) error {
	host, _, err := net.SplitHostPort(cfg.ServerAddr)
	if err != nil {
		return fmt.Errorf("invalid server address %q: %w", cfg.ServerAddr, err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	// tls.Dialer completes the TLS handshake (and our certificate checks)
	// before returning, so a bad pin fails here, before the token is sent.
	d := tls.Dialer{Config: tlsutil.ClientConfig(host, cfg.Fingerprint)}
	conn, err := d.DialContext(dialCtx, "tcp", cfg.ServerAddr)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", cfg.ServerAddr, err)
	}

	resp, err := handshake(conn, cfg)
	if err != nil {
		conn.Close()
		return err
	}

	session, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		conn.Close()
		return err
	}
	defer session.Close()

	onReady(resp)

	// Closing the session makes Serve return when ctx is cancelled.
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	err = srv.Serve(session)
	return fmt.Errorf("connection to server lost: %w", err)
}

func handshake(conn net.Conn, cfg Config) (*proto.HelloResponse, error) {
	conn.SetDeadline(time.Now().Add(proto.HandshakeTimeout))
	defer conn.SetDeadline(time.Time{})

	hello := &proto.Hello{
		Version:       proto.Version,
		Type:          proto.TunnelHTTP,
		Subdomain:     cfg.Subdomain,
		Token:         cfg.Token,
		ClientVersion: version.Version,
	}
	if err := proto.WriteHello(conn, hello); err != nil {
		return nil, fmt.Errorf("send hello: %w", err)
	}
	resp, err := proto.ReadHelloResponse(conn)
	if err != nil {
		return nil, fmt.Errorf("read hello response: %w", err)
	}
	if err := resp.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

// permanent reports whether retrying cannot help.
func permanent(err error, connectedBefore bool) bool {
	var he *proto.HandshakeError
	if errors.As(err, &he) {
		switch he.Code {
		case proto.ErrCodeInternal:
			return false
		case proto.ErrCodeSubdomainTaken:
			// After a drop the server may still hold our old session until
			// its keepalive notices; wait for it to let go.
			return !connectedBefore
		default:
			return true
		}
	}
	var uv *tlsutil.UnverifiedError
	return errors.As(err, &uv) || errors.Is(err, tlsutil.ErrFingerprintMismatch)
}

// backoff doubles the delay per attempt up to max, with ±20% jitter so many
// clients dropped by the same server restart do not reconnect in lockstep.
func backoff(attempt int, min, max time.Duration) time.Duration {
	d := max
	if attempt < 30 && min<<attempt < max {
		d = min << attempt
	}
	jitter := time.Duration(rand.Int64N(int64(d)/5*2+1)) - d/5
	return d + jitter
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.KeepAliveInterval = 10 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}
