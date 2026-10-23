// Command ochqd is the OCHQ tunnel server.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mukhiddinov/ochq/internal/server"
	"github.com/mukhiddinov/ochq/internal/tlsutil"
	"github.com/mukhiddinov/ochq/pkg/version"
)

func main() {
	tunnelAddr := flag.String("tunnel-addr", ":9000", "address for ochq client connections")
	httpAddr := flag.String("http-addr", ":8080", "address for public HTTP traffic (behind Caddy/Nginx)")
	domain := flag.String("domain", "localhost", "base domain; tunnels are served at <sub>.<domain>")
	scheme := flag.String("public-scheme", "http", "scheme of public URLs reported to clients")
	publicPort := flag.Int("public-port", 8080, "port in public URLs; 0 omits it (e.g. https on 443)")
	token := flag.String("token", os.Getenv("OCHQD_TOKEN"), "require clients to send this token [$OCHQD_TOKEN]")
	tlsCert := flag.String("tls-cert", os.Getenv("OCHQD_TLS_CERT"), "tunnel TLS certificate, e.g. from Let's Encrypt [$OCHQD_TLS_CERT]")
	tlsKey := flag.String("tls-key", os.Getenv("OCHQD_TLS_KEY"), "tunnel TLS private key [$OCHQD_TLS_KEY]")
	tlsDir := flag.String("tls-dir", os.Getenv("OCHQD_TLS_DIR"), "keep a self-signed tunnel certificate here (created on first start) [$OCHQD_TLS_DIR]")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ochqd", version.Version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cert, err := loadTLS(*tlsCert, *tlsKey, *tlsDir, logger)
	if err != nil {
		logger.Error("tunnel TLS", "err", err)
		os.Exit(1)
	}
	logger.Info("tunnel TLS ready", "fingerprint", tlsutil.Fingerprint(cert.Leaf))

	srv := server.New(server.Config{
		Domain:       *domain,
		PublicScheme: *scheme,
		PublicPort:   *publicPort,
		Token:        *token,
		TLS:          tlsutil.ServerConfig(cert),
		Logger:       logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.ListenAndServe(ctx, *tunnelAddr, *httpAddr); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// loadTLS picks the tunnel certificate: an explicit cert/key pair, else a
// persistent self-signed one in dir, else a throwaway one for development.
func loadTLS(certPath, keyPath, dir string, log *slog.Logger) (tls.Certificate, error) {
	switch {
	case certPath != "" || keyPath != "":
		return tlsutil.Load(certPath, keyPath)
	case dir != "":
		return tlsutil.LoadOrCreate(dir)
	default:
		log.Warn("no -tls-dir or -tls-cert: using an ephemeral certificate; its fingerprint changes on every restart")
		return tlsutil.SelfSigned()
	}
}
