package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mukhiddinov/ochq/internal/proto"
	"github.com/mukhiddinov/ochq/internal/server"
	"github.com/mukhiddinov/ochq/internal/tlsutil"
)

// startServer runs ochqd with a self-signed TLS certificate on random
// loopback ports. It returns the tunnel address, the public HTTP base URL
// and the certificate fingerprint clients must pin.
func startServer(t *testing.T, ctx context.Context) (tunnelAddr, httpURL, fingerprint string) {
	t.Helper()
	cert, err := tlsutil.SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	tunnelLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	httpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(server.Config{Domain: "ochq.test", TLS: tlsutil.ServerConfig(cert)})
	go srv.Serve(ctx, tunnelLn, httpLn)
	return tunnelLn.Addr().String(), "http://" + httpLn.Addr().String(), tlsutil.Fingerprint(cert.Leaf)
}

// startClient runs the tunnel and waits until it is ready.
func startClient(t *testing.T, ctx context.Context, cfg Config) (*proto.HelloResponse, <-chan error) {
	t.Helper()
	ready := make(chan *proto.HelloResponse, 1)
	errc := make(chan error, 1)
	cfg.OnReady = func(r *proto.HelloResponse) { ready <- r }
	go func() { errc <- Run(ctx, cfg) }()

	select {
	case resp := <-ready:
		return resp, errc
	case err := <-errc:
		t.Fatalf("client exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tunnel")
	}
	return nil, nil
}

// get sends a request to the public endpoint as if it came for host.
func get(t *testing.T, method, url, host, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTunnelEndToEnd(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fmt.Fprintf(w, "%s %s host=%s body=%s", r.Method, r.URL.RequestURI(), r.Host, body)
	}))
	defer local.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, httpURL, fp := startServer(t, ctx)

	resp, errc := startClient(t, ctx, Config{
		ServerAddr:  tunnelAddr,
		LocalAddr:   local.Listener.Addr().String(),
		Subdomain:   "demo",
		Fingerprint: fp,
	})
	if resp.Subdomain != "demo" || resp.PublicURL != "http://demo.ochq.test" {
		t.Fatalf("unexpected hello response %+v", resp)
	}

	code, body := get(t, "POST", httpURL+"/api/v1?x=1", "demo.ochq.test", "payload")
	if want := "POST /api/v1?x=1 host=demo.ochq.test body=payload"; code != 200 || body != want {
		t.Fatalf("got %d %q, want 200 %q", code, body, want)
	}

	// Many parallel requests share one TCP connection via yamux streams.
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			path := fmt.Sprintf("/req/%d", i)
			if code, body := get(t, "GET", httpURL+path, "demo.ochq.test", ""); code != 200 || !strings.Contains(body, path) {
				t.Errorf("%s: got %d %q", path, code, body)
			}
		})
	}
	wg.Wait()

	if code, _ := get(t, "GET", httpURL+"/", "nope.ochq.test", ""); code != http.StatusNotFound {
		t.Errorf("unknown subdomain: got %d, want 404", code)
	}

	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("Run after cancel: %v", err)
	}
}

func TestLocalServiceDown(t *testing.T) {
	// Grab a free port and close it so nothing listens there.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, httpURL, fp := startServer(t, ctx)
	resp, _ := startClient(t, ctx, Config{ServerAddr: tunnelAddr, LocalAddr: deadAddr, Fingerprint: fp})

	host := resp.Subdomain + ".ochq.test"
	code, body := get(t, "GET", httpURL+"/", host, "")
	if code != http.StatusBadGateway || !strings.Contains(body, "could not reach local service") {
		t.Fatalf("got %d %q, want 502 from client", code, body)
	}
}

func TestSubdomainTaken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, _, fp := startServer(t, ctx)
	startClient(t, ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1", Subdomain: "demo", Fingerprint: fp})

	err := Run(ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1", Subdomain: "demo", Fingerprint: fp})
	if err == nil || !strings.Contains(err.Error(), string(proto.ErrCodeSubdomainTaken)) {
		t.Fatalf("got %v, want subdomain_taken", err)
	}
}

func TestTLSVerification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, _, fp := startServer(t, ctx)

	other, err := tlsutil.SelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	// A different pin is what an interception would look like.
	err = Run(ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1", Fingerprint: tlsutil.Fingerprint(other.Leaf)})
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("wrong pin: got %v", err)
	}

	// No pin: the error must tell the user which fingerprint to verify.
	err = Run(ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1"})
	var uv *tlsutil.UnverifiedError
	if !errors.As(err, &uv) || uv.Fingerprint != fp {
		t.Fatalf("no pin: got %v, want UnverifiedError with %s", err, fp)
	}
}

// A plaintext client must not get anywhere near the handshake.
func TestPlaintextClientRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, _, _ := startServer(t, ctx)

	conn, err := net.Dial("tcp", tunnelAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := proto.WriteHello(conn, &proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP}); err != nil {
		t.Fatal(err)
	}
	if _, err := proto.ReadHelloResponse(conn); err == nil {
		t.Fatal("plaintext client completed the handshake")
	}
}
