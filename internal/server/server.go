// Package server implements ochqd: it accepts tunnels from ochq clients
// and routes public HTTP traffic into them by subdomain.
package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/mukhiddinov/ochq/internal/proto"
)

type Config struct {
	// Domain is the base domain tunnels live under, e.g. "ochq.uz".
	Domain string
	// PublicScheme and PublicPort build the URL reported to clients.
	// PublicPort 0 omits the port (the normal case behind Caddy on 443).
	PublicScheme string
	PublicPort   int
	// Token, if set, must match Hello.Token.
	Token string
	// TLS secures the tunnel listener. Nil means plaintext (tests only).
	TLS    *tls.Config
	Logger *slog.Logger
}

type Server struct {
	cfg      Config
	registry *Registry
	log      *slog.Logger
}

func New(cfg Config) *Server {
	cfg.Domain = strings.ToLower(strings.Trim(cfg.Domain, "."))
	if cfg.PublicScheme == "" {
		cfg.PublicScheme = "http"
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	return &Server{cfg: cfg, registry: NewRegistry(), log: cfg.Logger}
}

// ListenAndServe listens on tunnelAddr (for ochq clients) and httpAddr
// (for public traffic) and serves until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, tunnelAddr, httpAddr string) error {
	tunnelLn, err := net.Listen("tcp", tunnelAddr)
	if err != nil {
		return err
	}
	httpLn, err := net.Listen("tcp", httpAddr)
	if err != nil {
		tunnelLn.Close()
		return err
	}
	s.log.Info("listening", "tunnel", tunnelLn.Addr().String(), "http", httpLn.Addr().String(), "domain", s.cfg.Domain)
	return s.Serve(ctx, tunnelLn, httpLn)
}

// Serve is like ListenAndServe but uses the given listeners.
func (s *Server) Serve(ctx context.Context, tunnelLn, httpLn net.Listener) error {
	if s.cfg.TLS != nil {
		// The TLS handshake runs lazily on the first read, i.e. inside
		// handshake() and under its deadline, so a client that connects and
		// stalls cannot block the accept loop.
		tunnelLn = tls.NewListener(tunnelLn, s.cfg.TLS)
	}
	httpSrv := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 2)
	go func() { errc <- s.acceptTunnels(tunnelLn) }()
	go func() { errc <- httpSrv.Serve(httpLn) }()

	var err error
	select {
	case <-ctx.Done():
	case err = <-errc:
	}

	s.log.Info("shutting down")
	tunnelLn.Close()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
	s.registry.CloseAll()
	return err
}

func (s *Server) acceptTunnels(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleTunnel(conn)
	}
}

// handleTunnel runs for the whole life of one client connection.
func (s *Server) handleTunnel(conn net.Conn) {
	log := s.log.With("remote", conn.RemoteAddr().String())

	t, err := s.handshake(conn)
	if err != nil {
		log.Warn("handshake failed", "err", err)
		conn.Close()
		return
	}

	log = log.With("subdomain", t.subdomain)
	log.Info("tunnel opened")

	// yamux closes the session when the client disconnects or stops
	// answering keepalive pings.
	<-t.session.CloseChan()
	s.registry.Remove(t.subdomain, t)
	t.close()
	log.Info("tunnel closed")
}

// handshake reads Hello, claims a subdomain, answers, and switches the
// connection to yamux.
func (s *Server) handshake(conn net.Conn) (*tunnel, error) {
	conn.SetDeadline(time.Now().Add(proto.HandshakeTimeout))

	hello, err := proto.ReadHello(conn)
	if err != nil {
		return nil, fmt.Errorf("read hello: %w", err)
	}

	sub, herr := s.claim(hello)
	if herr != nil {
		proto.WriteHelloResponse(conn, &proto.HelloResponse{Code: herr.Code, Error: herr.Message})
		return nil, herr
	}

	resp := &proto.HelloResponse{Subdomain: sub, PublicURL: s.publicURL(sub)}
	if err := proto.WriteHelloResponse(conn, resp); err != nil {
		s.registry.Remove(sub, nil)
		return nil, fmt.Errorf("write hello response: %w", err)
	}

	// From here on the connection belongs to yamux, which has its own
	// keepalive and write timeouts.
	conn.SetDeadline(time.Time{})
	session, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		s.registry.Remove(sub, nil)
		return nil, err
	}

	t := newTunnel(sub, session)
	s.registry.Activate(sub, t)
	return t, nil
}

// claim validates hello and reserves a subdomain for it.
func (s *Server) claim(hello *proto.Hello) (string, *proto.HandshakeError) {
	reject := func(code proto.ErrorCode, format string, args ...any) (string, *proto.HandshakeError) {
		return "", &proto.HandshakeError{Code: code, Message: fmt.Sprintf(format, args...)}
	}

	if hello.Version != proto.Version {
		return reject(proto.ErrCodeVersionMismatch, "server speaks protocol v%d, client sent v%d", proto.Version, hello.Version)
	}
	if hello.Type != proto.TunnelHTTP {
		return reject(proto.ErrCodeBadRequest, "unsupported tunnel type %q", hello.Type)
	}
	if s.cfg.Token != "" && hello.Token == "" {
		return reject(proto.ErrCodeUnauthorized, "this server requires a token")
	}
	if s.cfg.Token != "" && subtle.ConstantTimeCompare([]byte(hello.Token), []byte(s.cfg.Token)) != 1 {
		return reject(proto.ErrCodeUnauthorized, "invalid token")
	}

	if hello.Subdomain == "" {
		sub, err := s.registry.ReserveRandom()
		if err != nil {
			return reject(proto.ErrCodeInternal, "%v", err)
		}
		return sub, nil
	}

	sub, err := proto.NormalizeSubdomain(hello.Subdomain)
	if err != nil {
		return reject(proto.ErrCodeInvalidSubdomain, "invalid subdomain %q", hello.Subdomain)
	}
	if isReserved(sub) {
		return reject(proto.ErrCodeInvalidSubdomain, "subdomain %q is reserved", sub)
	}
	if !s.registry.Reserve(sub) {
		return reject(proto.ErrCodeSubdomainTaken, "subdomain %q is already in use", sub)
	}
	return sub, nil
}

func (s *Server) publicURL(sub string) string {
	host := sub + "." + s.cfg.Domain
	if s.cfg.PublicPort != 0 {
		host = net.JoinHostPort(host, strconv.Itoa(s.cfg.PublicPort))
	}
	return s.cfg.PublicScheme + "://" + host
}

func yamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	// Ping every 10s so a dead client frees its subdomain quickly.
	cfg.KeepAliveInterval = 10 * time.Second
	cfg.LogOutput = io.Discard
	return cfg
}
