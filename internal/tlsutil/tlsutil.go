// Package tlsutil provides TLS for the tunnel port (:9000).
//
// Until the server has a CA-signed certificate (Let's Encrypt for a real
// domain), ochqd uses a self-signed one and clients pin its public key
// fingerprint — the same trust model as SSH host keys.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	certFile = "cert.pem"
	keyFile  = "key.pem"
)

// Fingerprint returns "SHA256:<base64>" of the certificate's public key
// (SubjectPublicKeyInfo). Hashing the key rather than the whole certificate
// keeps the pin stable if the certificate is re-issued with the same key.
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// SelfSigned generates a new ECDSA P-256 self-signed certificate.
func SelfSigned() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "ochqd"},
		NotBefore:    time.Now().Add(-time.Hour),
		// Trust comes from the pinned key, not from expiry, so make it long.
		NotAfter:    time.Now().AddDate(10, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, nil
}

// LoadOrCreate loads cert.pem/key.pem from dir, creating a self-signed pair
// on first use so the fingerprint survives restarts.
func LoadOrCreate(dir string) (tls.Certificate, error) {
	certPath, keyPath := filepath.Join(dir, certFile), filepath.Join(dir, keyFile)

	cert, err := Load(certPath, keyPath)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return cert, err
	}

	cert, err = SelfSigned()
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return tls.Certificate{}, err
	}
	if err := writePEM(keyPath, "PRIVATE KEY", keyDER, 0o600); err != nil {
		return tls.Certificate{}, err
	}
	if err := writePEM(certPath, "CERTIFICATE", cert.Certificate[0], 0o644); err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

// Load reads a PEM certificate chain and key, e.g. from Let's Encrypt.
func Load(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return tls.Certificate{}, err
	}
	cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
	return cert, err
}

func writePEM(path, typ string, der []byte, mode os.FileMode) error {
	data := pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
	return os.WriteFile(path, data, mode)
}

// ServerConfig is the TLS config for the ochqd tunnel listener.
func ServerConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		// We ship both ends, so there is no old client to stay compatible with.
		MinVersion: tls.VersionTLS13,
	}
}

// UnverifiedError means the server certificate is not signed by a trusted
// CA and no fingerprint was pinned. It carries the presented fingerprint so
// the user can compare it with the one ochqd logs.
type UnverifiedError struct {
	Fingerprint string
	Err         error
}

func (e *UnverifiedError) Error() string {
	return fmt.Sprintf("server certificate is not trusted (%v); if this server uses a "+
		"self-signed certificate, verify its fingerprint with the server admin "+
		"and pass --fingerprint %s", e.Err, e.Fingerprint)
}

func (e *UnverifiedError) Unwrap() error { return e.Err }

// ClientConfig verifies the server either by pinned fingerprint (if pin is
// set) or by the system's trusted CAs for serverName.
func ClientConfig(serverName, pin string) *tls.Config {
	return &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
		// Go's built-in verification cannot do pinning, so we turn it off
		// and do all verification ourselves in VerifyConnection, which runs
		// on every handshake.
		InsecureSkipVerify: true,
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("server sent no certificate")
			}
			leaf := cs.PeerCertificates[0]
			got := Fingerprint(leaf)

			if pin != "" {
				if got != pin {
					return fmt.Errorf("server fingerprint mismatch: got %s, want %s "+
						"(the server key changed, or someone is intercepting the connection)", got, pin)
				}
				return nil
			}

			opts := x509.VerifyOptions{DNSName: serverName, Intermediates: x509.NewCertPool()}
			for _, c := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(c)
			}
			if _, err := leaf.Verify(opts); err != nil {
				return &UnverifiedError{Fingerprint: got, Err: err}
			}
			return nil
		},
	}
}
