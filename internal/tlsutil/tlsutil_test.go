package tlsutil

import (
	"crypto/tls"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrCreateIsStable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")

	first, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if Fingerprint(first.Leaf) != Fingerprint(second.Leaf) {
		t.Fatal("fingerprint changed between loads")
	}

	info, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestFingerprintFormat(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	fp := Fingerprint(cert.Leaf)
	if !strings.HasPrefix(fp, "SHA256:") || len(fp) != len("SHA256:")+43 {
		t.Fatalf("unexpected fingerprint %q", fp)
	}
}

// handshake runs a TLS handshake over a pipe and returns the client error.
func handshake(t *testing.T, server tls.Certificate, client *tls.Config) error {
	t.Helper()
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	go func() {
		srv := tls.Server(s, ServerConfig(server))
		srv.Handshake()
		srv.Close()
	}()
	return tls.Client(c, client).Handshake()
}

func TestClientPinning(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	other, err := SelfSigned()
	if err != nil {
		t.Fatal(err)
	}

	if err := handshake(t, cert, ClientConfig("ochqd", Fingerprint(cert.Leaf))); err != nil {
		t.Fatalf("correct pin rejected: %v", err)
	}

	err = handshake(t, cert, ClientConfig("ochqd", Fingerprint(other.Leaf)))
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("wrong pin: got %v, want fingerprint mismatch", err)
	}
}

func TestClientWithoutPinRejectsSelfSigned(t *testing.T) {
	cert, err := SelfSigned()
	if err != nil {
		t.Fatal(err)
	}
	err = handshake(t, cert, ClientConfig("ochqd", ""))

	var uv *UnverifiedError
	if !errors.As(err, &uv) {
		t.Fatalf("got %v, want UnverifiedError", err)
	}
	if uv.Fingerprint != Fingerprint(cert.Leaf) {
		t.Fatalf("error carries %s, want %s", uv.Fingerprint, Fingerprint(cert.Leaf))
	}
}
