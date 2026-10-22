package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/hashicorp/yamux"
)

// tunnel is one connected ochq client.
type tunnel struct {
	subdomain string
	session   *yamux.Session
	transport *http.Transport
	proxy     *httputil.ReverseProxy
}

func newTunnel(sub string, session *yamux.Session) *tunnel {
	// Every "TCP connection" the transport dials is really a new yamux
	// stream inside the client's tunnel. The transport pools and reuses
	// them for keep-alive like ordinary connections.
	transport := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			return session.Open()
		},
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = pr.In.Host
			pr.Out.Host = pr.In.Host

			// Keep the chain set by Caddy/Nginx in front of us and append
			// the peer address, like a classic reverse proxy does.
			pr.Out.Header["X-Forwarded-For"] = pr.In.Header["X-Forwarded-For"]
			pr.SetXForwarded()
			if proto := pr.In.Header.Get("X-Forwarded-Proto"); proto != "" {
				pr.Out.Header.Set("X-Forwarded-Proto", proto)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "ochq: tunnel unavailable", http.StatusBadGateway)
		},
	}

	return &tunnel{subdomain: sub, session: session, transport: transport, proxy: proxy}
}

func (t *tunnel) close() {
	t.transport.CloseIdleConnections()
	t.session.Close()
}

// ServeHTTP routes a public request to the tunnel named by its Host header.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sub, ok := s.subdomainFromHost(r.Host)
	if !ok {
		http.Error(w, "ochq: unknown host", http.StatusNotFound)
		return
	}
	t, ok := s.registry.Lookup(sub)
	if !ok {
		http.Error(w, "ochq: tunnel "+sub+" not found", http.StatusNotFound)
		return
	}
	t.proxy.ServeHTTP(w, r)
}

// subdomainFromHost extracts "x8k2" from "x8k2.ochq.uz" or "x8k2.ochq.uz:8080".
func (s *Server) subdomainFromHost(host string) (string, bool) {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	sub, ok := strings.CutSuffix(host, "."+s.cfg.Domain)
	if !ok || sub == "" || strings.Contains(sub, ".") {
		return "", false
	}
	return sub, true
}
