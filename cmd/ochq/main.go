// Command ochq exposes a local port through an ochqd server.
//
//	ochq 3000
//	ochq http 8080
//	ochq http 3000 --sub my-api --server tunnel.ochq.uz:9000
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/mukhiddinov/ochq/internal/client"
	"github.com/mukhiddinov/ochq/internal/proto"
	"github.com/mukhiddinov/ochq/pkg/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ochq:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("ochq", flag.ContinueOnError)
	server := fs.String("server", envOr("OCHQ_SERVER", "localhost:9000"), "ochqd tunnel address (host:port) [$OCHQ_SERVER]")
	sub := fs.String("sub", "", "requested subdomain (random if empty)")
	token := fs.String("token", os.Getenv("OCHQ_TOKEN"), "auth token, if the server requires one [$OCHQ_TOKEN]")
	fingerprint := fs.String("fingerprint", os.Getenv("OCHQ_FINGERPRINT"), "pin the server's self-signed TLS key, e.g. SHA256:... [$OCHQ_FINGERPRINT]")
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: ochq [http] <port> [flags]\n\nFlags:\n")
		fs.PrintDefaults()
	}

	positional, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if *showVersion {
		fmt.Println("ochq", version.Version)
		return nil
	}

	if len(positional) > 0 && positional[0] == "http" {
		positional = positional[1:]
	}
	if len(positional) != 1 {
		fs.Usage()
		return errors.New("expected exactly one port, e.g. `ochq 3000`")
	}
	localAddr, err := parseLocalAddr(positional[0])
	if err != nil {
		return err
	}

	// Validate early for a clear message instead of a server round-trip.
	if *sub != "" {
		normalized, err := proto.NormalizeSubdomain(*sub)
		if err != nil {
			return fmt.Errorf("invalid --sub %q: use 3-63 letters, digits or hyphens", *sub)
		}
		*sub = normalized
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Connecting to %s ...\n", *server)
	return client.Run(ctx, client.Config{
		ServerAddr:  *server,
		LocalAddr:   localAddr,
		Subdomain:   *sub,
		Token:       *token,
		Fingerprint: *fingerprint,
		OnReady: func(resp *proto.HelloResponse) {
			fmt.Printf("Forwarding  %s -> http://%s\n", resp.PublicURL, localAddr)
			fmt.Println("Press Ctrl+C to stop.")
		},
	})
}

// parseInterspersed lets flags appear before and after positional args.
// The standard flag package stops at the first positional argument, so we
// collect it and keep parsing the rest.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

// parseLocalAddr accepts "3000" or "host:3000".
func parseLocalAddr(s string) (string, error) {
	host, portStr := "127.0.0.1", s
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, portStr = h, p
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("invalid port %q", s)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
