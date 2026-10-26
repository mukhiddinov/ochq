package client

import (
	"bufio"
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
		Subdomain:   "myapp",
		Fingerprint: fp,
	})
	if resp.Subdomain != "myapp" || resp.PublicURL != "http://myapp.ochq.test" {
		t.Fatalf("unexpected hello response %+v", resp)
	}

	code, body := get(t, "POST", httpURL+"/api/v1?x=1", "myapp.ochq.test", "payload")
	if want := "POST /api/v1?x=1 host=myapp.ochq.test body=payload"; code != 200 || body != want {
		t.Fatalf("got %d %q, want 200 %q", code, body, want)
	}

	// Many parallel requests share one TCP connection via yamux streams.
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			path := fmt.Sprintf("/req/%d", i)
			if code, body := get(t, "GET", httpURL+path, "myapp.ochq.test", ""); code != 200 || !strings.Contains(body, path) {
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
	startClient(t, ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1", Subdomain: "myapp", Fingerprint: fp})

	err := Run(ctx, Config{ServerAddr: tunnelAddr, LocalAddr: "127.0.0.1:1", Subdomain: "myapp", Fingerprint: fp})
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

func TestRequestLog(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, "ok")
	}))
	defer local.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, httpURL, fp := startServer(t, ctx)

	logs := make(chan RequestLog, 10)
	resp, _ := startClient(t, ctx, Config{
		ServerAddr: tunnelAddr, LocalAddr: local.Listener.Addr().String(), Fingerprint: fp,
		OnRequest: func(l RequestLog) { logs <- l },
	})
	host := resp.Subdomain + ".ochq.test"

	get(t, "POST", httpURL+"/api/v1/login?next=/", host, "x")
	get(t, "GET", httpURL+"/missing", host, "")

	for _, want := range []RequestLog{
		{Method: "POST", Path: "/api/v1/login?next=/", Status: 200},
		{Method: "GET", Path: "/missing", Status: 404},
	} {
		select {
		case got := <-logs:
			if got.Method != want.Method || got.Path != want.Path || got.Status != want.Status || got.Duration <= 0 {
				t.Errorf("got %+v, want %+v", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no log for %s %s", want.Method, want.Path)
		}
	}
}

// Upgrades (WebSocket) must pass through both proxies and be logged as 101.
func TestUpgradeEcho(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "echo" {
			http.Error(w, "want upgrade", http.StatusBadRequest)
			return
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n")
		brw.Flush()
		io.Copy(conn, brw)
	}))
	defer local.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tunnelAddr, httpURL, fp := startServer(t, ctx)
	logs := make(chan RequestLog, 1)
	resp, _ := startClient(t, ctx, Config{
		ServerAddr: tunnelAddr, LocalAddr: local.Listener.Addr().String(), Fingerprint: fp,
		OnRequest: func(l RequestLog) { logs <- l },
	})

	conn, err := net.Dial("tcp", strings.TrimPrefix(httpURL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(conn, "GET /ws HTTP/1.1\r\nHost: %s.ochq.test\r\nUpgrade: echo\r\nConnection: Upgrade\r\n\r\n", resp.Subdomain)

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil || !strings.Contains(status, "101") {
		t.Fatalf("status line %q, %v", status, err)
	}
	for line := status; line != "\r\n"; {
		if line, err = br.ReadString('\n'); err != nil {
			t.Fatal(err)
		}
	}
	io.WriteString(conn, "ping\n")
	if echo, err := br.ReadString('\n'); echo != "ping\n" {
		t.Fatalf("echo %q, %v", echo, err)
	}

	conn.Close()
	select {
	case l := <-logs:
		if l.Status != http.StatusSwitchingProtocols {
			t.Fatalf("logged status %d, want 101", l.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upgrade not logged")
	}
}

// When the server goes away and comes back on the same address, the client
// reconnects on its own and keeps its subdomain.
func TestReconnectKeepsSubdomain(t *testing.T) {
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "still here")
	}))
	defer local.Close()

	cert, err := tlsutil.SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	tunnelLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpLn, _ := net.Listen("tcp", "127.0.0.1:0")
	tunnelAddr, httpAddr := tunnelLn.Addr().String(), httpLn.Addr().String()
	serve := func(ctx context.Context, tl, hl net.Listener) {
		srv := server.New(server.Config{Domain: "ochq.test", TLS: tlsutil.ServerConfig(cert)})
		go srv.Serve(ctx, tl, hl)
	}
	srvCtx, stopServer := context.WithCancel(context.Background())
	serve(srvCtx, tunnelLn, httpLn)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan *proto.HelloResponse, 4)
	reconnects := make(chan error, 10)
	go Run(ctx, Config{
		ServerAddr: tunnelAddr, LocalAddr: local.Listener.Addr().String(),
		Fingerprint: tlsutil.Fingerprint(cert.Leaf),
		MinBackoff:  50 * time.Millisecond, MaxBackoff: 200 * time.Millisecond,
		OnReady:     func(r *proto.HelloResponse) { ready <- r },
		OnReconnect: func(_ int, _ time.Duration, err error) { reconnects <- err },
	})
	first := <-ready // random subdomain

	stopServer()
	select {
	case <-reconnects:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not notice the server going away")
	}

	// Bring the server back on the same ports.
	time.Sleep(100 * time.Millisecond)
	tunnelLn, err = net.Listen("tcp", tunnelAddr)
	if err != nil {
		t.Fatal(err)
	}
	httpLn, err = net.Listen("tcp", httpAddr)
	if err != nil {
		t.Fatal(err)
	}
	srvCtx, stopServer = context.WithCancel(context.Background())
	defer stopServer()
	serve(srvCtx, tunnelLn, httpLn)

	select {
	case second := <-ready:
		if second.Subdomain != first.Subdomain {
			t.Fatalf("subdomain changed: %s -> %s", first.Subdomain, second.Subdomain)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client did not reconnect")
	}
	if code, body := get(t, "GET", "http://"+httpAddr+"/", first.Subdomain+".ochq.test", ""); code != 200 || body != "still here" {
		t.Fatalf("after reconnect: %d %q", code, body)
	}
}

// Retrying cannot fix a wrong token: Run must give up at once.
func TestPermanentErrorNoRetry(t *testing.T) {
	cert, _ := tlsutil.SelfSigned()
	tunnelLn, _ := net.Listen("tcp", "127.0.0.1:0")
	httpLn, _ := net.Listen("tcp", "127.0.0.1:0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := server.New(server.Config{Domain: "ochq.test", Token: "right", TLS: tlsutil.ServerConfig(cert)})
	go srv.Serve(ctx, tunnelLn, httpLn)

	retried := false
	err := Run(ctx, Config{
		ServerAddr: tunnelLn.Addr().String(), LocalAddr: "127.0.0.1:1", Token: "wrong",
		Fingerprint: tlsutil.Fingerprint(cert.Leaf),
		OnReconnect: func(int, time.Duration, error) { retried = true },
	})
	var he *proto.HandshakeError
	if !errors.As(err, &he) || he.Code != proto.ErrCodeUnauthorized || retried {
		t.Fatalf("got %v (retried=%v), want immediate unauthorized", err, retried)
	}
}

func TestBackoff(t *testing.T) {
	min, max := time.Second, 30*time.Second
	for attempt, base := range []time.Duration{1, 2, 4, 8, 16, 30, 30} {
		base *= time.Second
		for range 50 {
			d := backoff(attempt, min, max)
			if d < base*8/10 || d > base*12/10 {
				t.Fatalf("attempt %d: %v outside %v±20%%", attempt, d, base)
			}
		}
	}
	if d := backoff(1000, min, max); d > max*12/10 {
		t.Fatalf("huge attempt overflowed: %v", d)
	}
}
