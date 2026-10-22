// Command ochqd is the OCHQ tunnel server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mukhiddinov/ochq/internal/server"
	"github.com/mukhiddinov/ochq/pkg/version"
)

func main() {
	tunnelAddr := flag.String("tunnel-addr", ":9000", "address for ochq client connections")
	httpAddr := flag.String("http-addr", ":8080", "address for public HTTP traffic (behind Caddy/Nginx)")
	domain := flag.String("domain", "localhost", "base domain; tunnels are served at <sub>.<domain>")
	scheme := flag.String("public-scheme", "http", "scheme of public URLs reported to clients")
	publicPort := flag.Int("public-port", 8080, "port in public URLs; 0 omits it (e.g. https on 443)")
	token := flag.String("token", os.Getenv("OCHQD_TOKEN"), "require clients to send this token [$OCHQD_TOKEN]")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ochqd", version.Version)
		return
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := server.New(server.Config{
		Domain:       *domain,
		PublicScheme: *scheme,
		PublicPort:   *publicPort,
		Token:        *token,
		Logger:       logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.ListenAndServe(ctx, *tunnelAddr, *httpAddr); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
