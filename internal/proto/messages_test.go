package proto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
)

func TestHelloRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := &Hello{Version: Version, Type: TunnelHTTP, Subdomain: "my-api", Token: "secret", ClientVersion: "1.0.0"}

	if err := WriteHello(&buf, want); err != nil {
		t.Fatalf("WriteHello: %v", err)
	}
	got, err := ReadHello(&buf)
	if err != nil {
		t.Fatalf("ReadHello: %v", err)
	}
	if *got != *want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestHelloResponseRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := &HelloResponse{Subdomain: "x8k2", PublicURL: "https://x8k2.ochq.uz"}

	if err := WriteHelloResponse(&buf, want); err != nil {
		t.Fatalf("WriteHelloResponse: %v", err)
	}
	got, err := ReadHelloResponse(&buf)
	if err != nil {
		t.Fatalf("ReadHelloResponse: %v", err)
	}
	if *got != *want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !got.OK() || got.Err() != nil {
		t.Fatalf("expected OK response, got err %v", got.Err())
	}
}

func TestHelloResponseErr(t *testing.T) {
	resp := &HelloResponse{Code: ErrCodeSubdomainTaken, Error: "subdomain my-api is already in use"}
	if resp.OK() {
		t.Fatal("expected rejected response")
	}
	var he *HandshakeError
	if !errors.As(resp.Err(), &he) || he.Code != ErrCodeSubdomainTaken {
		t.Fatalf("got %v, want HandshakeError with code %q", resp.Err(), ErrCodeSubdomainTaken)
	}
}

func TestReadWrongType(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHelloResponse(&buf, &HelloResponse{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadHello(&buf); !errors.Is(err, ErrUnexpectedType) {
		t.Fatalf("got %v, want ErrUnexpectedType", err)
	}
}

func TestReadFrameTooLarge(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], MaxFrameSize+1)
	if _, err := ReadHello(bytes.NewReader(hdr[:])); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("got %v, want ErrFrameTooLarge", err)
	}
}

func TestReadTruncated(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHello(&buf, &Hello{Version: Version, Type: TunnelHTTP}); err != nil {
		t.Fatal(err)
	}
	truncated := buf.Bytes()[:buf.Len()-3]
	if _, err := ReadHello(bytes.NewReader(truncated)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("got %v, want io.ErrUnexpectedEOF", err)
	}
}

// The reader must not consume bytes past the frame: they belong to yamux.
func TestReadDoesNotOverread(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteHello(&buf, &Hello{Version: Version, Type: TunnelHTTP}); err != nil {
		t.Fatal(err)
	}
	trailer := []byte("yamux-bytes")
	buf.Write(trailer)

	if _, err := ReadHello(&buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), trailer) {
		t.Fatalf("remaining %q, want %q", buf.Bytes(), trailer)
	}
}

func TestHandshakeOverPipe(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	errc := make(chan error, 1)
	go func() {
		h, err := ReadHello(server)
		if err != nil {
			errc <- err
			return
		}
		errc <- WriteHelloResponse(server, &HelloResponse{Subdomain: h.Subdomain, PublicURL: "https://" + h.Subdomain + ".ochq.uz"})
	}()

	if err := WriteHello(client, &Hello{Version: Version, Type: TunnelHTTP, Subdomain: "demo"}); err != nil {
		t.Fatal(err)
	}
	resp, err := ReadHelloResponse(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("server side: %v", err)
	}
	if resp.PublicURL != "https://demo.ochq.uz" {
		t.Fatalf("got %q", resp.PublicURL)
	}
}

func TestNormalizeSubdomain(t *testing.T) {
	valid := map[string]string{
		"my-api":   "my-api",
		"  X8K2  ": "x8k2",
		"abc":      "abc",
	}
	for in, want := range valid {
		got, err := NormalizeSubdomain(in)
		if err != nil || got != want {
			t.Errorf("NormalizeSubdomain(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	invalid := []string{"", "ab", "-api", "api-", "my_api", "my.api", "ап1", string(make([]byte, 64))}
	for _, in := range invalid {
		if _, err := NormalizeSubdomain(in); !errors.Is(err, ErrInvalidSubdomain) {
			t.Errorf("NormalizeSubdomain(%q) err = %v; want ErrInvalidSubdomain", in, err)
		}
	}
}
