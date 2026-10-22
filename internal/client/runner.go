// Package client implements the ochq side of a tunnel: it connects to
// ochqd and forwards every incoming stream to a local service.
package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/mukhiddinov/ochq/internal/proto"
	"github.com/mukhiddinov/ochq/pkg/version"
)

const (
	dialTimeout      = 10 * time.Second
	localDialTimeout = 5 * time.Second
)

type Config struct {
	ServerAddr string // ochqd tunnel address, e.g. "tunnel.ochq.uz:9000"
	LocalAddr  string // local service, e.g. "127.0.0.1:3000"
	Subdomain  string // requested subdomain; empty lets the server pick
	Token      string

	// OnReady, if set, is called once the tunnel is up.
	OnReady func(*proto.HelloResponse)
}

// Run opens a tunnel and serves it until ctx is cancelled (returns nil)
// or the connection to the server is lost (returns an error).
func Run(ctx context.Context, cfg Config) error {
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	var d net.Dialer
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

	if cfg.OnReady != nil {
		cfg.OnReady(resp)
	}

	// Closing the session unblocks Accept below when ctx is cancelled.
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()

	for {
		stream, err := session.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("tunnel closed: %w", err)
		}
		go forward(stream, cfg.LocalAddr)
	}
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

// forward pipes one tunnel stream to a fresh connection to the local service.
func forward(stream net.Conn, localAddr string) {
	defer stream.Close()

	local, err := net.DialTimeout("tcp", localAddr, localDialTimeout)
	if err != nil {
		writeBadGateway(stream, localAddr, err)
		return
	}
	defer local.Close()

	join(stream, local)
}

// join copies bytes both ways and returns as soon as either side is done;
// the caller's deferred Close calls then unblock the other direction.
func join(a, b net.Conn) {
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		io.Copy(dst, src)
		done <- struct{}{}
	}
	go pipe(a, b)
	go pipe(b, a)
	<-done
}

// writeBadGateway answers the waiting HTTP request directly, so the visitor
// sees why the page failed instead of a bare connection reset.
func writeBadGateway(w io.Writer, localAddr string, cause error) {
	body := fmt.Sprintf("ochq: could not reach local service at %s: %v\n", localAddr, cause)
	fmt.Fprintf(w, "HTTP/1.1 502 Bad Gateway\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\n"+
		"Content-Length: %d\r\n"+
		"Connection: close\r\n\r\n%s", len(body), body)
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.KeepAliveInterval = 10 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}
