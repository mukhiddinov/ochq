package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/mukhiddinov/ochq/internal/client"
	"github.com/mukhiddinov/ochq/internal/proto"
	"github.com/mukhiddinov/ochq/internal/tlsutil"
	"github.com/mukhiddinov/ochq/internal/ui"
	"github.com/mukhiddinov/ochq/pkg/version"
)

// The hosted service at ochq.uz works with no setup. Until accounts exist,
// every client shares one token: it ships in every binary, so it is public
// and only keeps out clients that were not built for this server. The
// fingerprint pins the server's self-signed tunnel key. Both apply only to
// defaultServer; for any other server they come from flags or environment.
const (
	defaultServer      = "tunnel.ochq.uz:9000"
	defaultToken       = "c99442c180ecc581c2b4549c6fd8e638fa2159877971f7dc"
	defaultFingerprint = "SHA256:zj3vhql3jds4zKqLCUGod9iyGiLn8vXjgy5z7sV1e7w"
)

// runFunc opens the tunnel; tests replace client.Run with a fake.
type runFunc func(context.Context, client.Config) error

type tunnelOptions struct {
	server      string
	token       string
	fingerprint string
	sub         string
}

const longHelp = `Expose a local port to the internet through an OCHQ tunnel.

ochq connects to an ochqd server and gives your local service a public URL.
Every request to that URL is carried back through the tunnel to the port you
name, and shows up in the live log.

The hosted server at ochq.uz needs no setup. To use your own ochqd server,
pass --server (and --token, --fingerprint if it needs them).

Environment variables (a flag given on the command line wins):
  OCHQ_SERVER        ochqd tunnel address, host:port
  OCHQ_TOKEN         access token, if the server requires one
  OCHQ_FINGERPRINT   pinned key of a self-signed server, SHA256:...
  NO_COLOR           disable colored output`

const examples = `  # Expose localhost:3000 on a random subdomain
  ochq 3000

  # Ask for a specific subdomain: https://my-api.ochq.uz
  ochq 3000 --sub my-api

  # Expose another machine on your network
  ochq 192.168.1.20:80

  # Use your own server
  ochq 3000 --server tunnel.example.com:9000`

func newRootCmd(run runFunc, out io.Writer) *cobra.Command {
	opts := &tunnelOptions{}

	root := &cobra.Command{
		Use:     "ochq [http] <port | host:port>",
		Short:   "Expose a local port to the internet through an OCHQ tunnel",
		Long:    longHelp,
		Example: examples,
		Version: version.Version,
		// `ochq 3000`: the port is the root command's argument, so ochq http
		// stays optional. An explicit Args validator also stops cobra from
		// treating every port as an unknown subcommand.
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: noFileCompletion,
		// main prints errors (with a hint) itself; usage is only noise there.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runTunnel(cmd, run, out, opts, args[0])
		},
	}
	root.SetVersionTemplate("ochq {{.Version}}\n")
	root.SetOut(out)
	// Cobra only sets this on its own unknown-command path; runTunnel asks
	// for suggestions directly, because to cobra `ochq verison` is a port.
	root.SuggestionsMinimumDistance = 2
	addTunnelFlags(root.Flags(), opts)

	httpCmd := &cobra.Command{
		Use:               "http <port | host:port>",
		Short:             "Expose a local HTTP service (the default)",
		Example:           "  ochq http 8080\n  ochq http 3000 --sub my-api",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: noFileCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTunnel(cmd, run, out, opts, args[0])
		},
	}
	addTunnelFlags(httpCmd.Flags(), opts)

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), "ochq", version.Version)
		},
	}

	root.AddCommand(httpCmd, versionCmd)
	return root
}

// addTunnelFlags registers the tunnel flags. Their defaults deliberately do
// not come from the environment: help prints defaults, and a token copied
// into a help screenshot would leak. Environment values are applied after
// parsing, in resolveEnv.
func addTunnelFlags(fs *pflag.FlagSet, o *tunnelOptions) {
	fs.StringVar(&o.server, "server", defaultServer, "ochqd tunnel address, host:port [$OCHQ_SERVER]")
	fs.StringVar(&o.token, "token", "", "access token, if the server requires one [$OCHQ_TOKEN]")
	fs.StringVar(&o.fingerprint, "fingerprint", "", "pin a self-signed server's key, SHA256:... [$OCHQ_FINGERPRINT]")
	fs.StringVar(&o.sub, "sub", "", "requested subdomain, e.g. my-api or my-api.ochq.uz (random if empty)")
}

// resolveEnv fills every flag the user did not set from its variable.
func resolveEnv(cmd *cobra.Command, o *tunnelOptions) {
	for _, f := range []struct {
		flag, env string
		val       *string
	}{
		{"server", "OCHQ_SERVER", &o.server},
		{"token", "OCHQ_TOKEN", &o.token},
		{"fingerprint", "OCHQ_FINGERPRINT", &o.fingerprint},
	} {
		if v := os.Getenv(f.env); v != "" && !cmd.Flags().Changed(f.flag) {
			*f.val = v
		}
	}
	if o.server == defaultServer {
		if o.token == "" {
			o.token = defaultToken
		}
		if o.fingerprint == "" {
			o.fingerprint = defaultFingerprint
		}
	}
}

// subdomainArg accepts the name alone or the full public host, so
// "--sub demo.ochq.uz" works like "--sub demo". The domain is the server's
// host minus its first label: tunnel.ochq.uz serves *.ochq.uz.
func subdomainArg(sub, server string) string {
	name, rest, ok := strings.Cut(strings.ToLower(sub), ".")
	if !ok {
		return sub
	}
	host, _, err := net.SplitHostPort(server)
	if err != nil {
		host = server
	}
	if _, domain, ok := strings.Cut(strings.ToLower(host), "."); ok && rest == domain {
		return name
	}
	return sub
}

func runTunnel(cmd *cobra.Command, run runFunc, out io.Writer, o *tunnelOptions, target string) error {
	resolveEnv(cmd, o)

	localAddr, err := parseLocalAddr(target)
	if err != nil {
		// `ochq verison`: a mistyped command lands here as the port.
		if s := cmd.Root().SuggestionsFor(target); len(s) > 0 {
			return fmt.Errorf("unknown command %q — did you mean %q?", target, s[0])
		}
		return err
	}

	sub := subdomainArg(o.sub, o.server)
	if sub != "" {
		if sub, err = proto.NormalizeSubdomain(sub); err != nil {
			return fmt.Errorf("invalid --sub %q: use 3-63 letters, digits or hyphens", o.sub)
		}
	}

	printer := ui.New(out)
	// Types the logo while the tunnel connects; later output waits for it.
	printer.AnimateBanner(cmd.Context(), version.Version)
	defer printer.Close()
	localURL := "http://" + displayAddr(localAddr)
	if !listening(localAddr) {
		printer.Warn(fmt.Sprintf("nothing is listening on %s yet — visitors get 502 until it starts", localURL))
	}
	printer.Connecting(o.server)

	return run(cmd.Context(), client.Config{
		ServerAddr:  o.server,
		LocalAddr:   localAddr,
		Subdomain:   sub,
		Token:       o.token,
		Fingerprint: o.fingerprint,
		OnReady: func(resp *proto.HelloResponse) {
			printer.Online(resp.PublicURL, localURL)
		},
		OnReconnect: printer.Reconnecting,
		OnRequest: func(r client.RequestLog) {
			printer.Request(r.Method, r.Path, r.Status, r.Duration)
		},
	})
}

func noFileCompletion(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// legacyFlag matches the single-dash long flags of ochq 0.1 (Go's flag
// package accepted -version, -sub, ...); pflag would read them as bundles of
// one-letter shorthands.
var legacyFlag = regexp.MustCompile(`^-(server|token|fingerprint|sub|version|help)(=.*)?$`)

// compatArgs rewrites those flags to their --long form so existing scripts
// and habits keep working.
func compatArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if legacyFlag.MatchString(a) {
			a = "-" + a
		}
		out[i] = a
	}
	return out
}

// hintFor suggests a fix for errors a user can act on.
func hintFor(err error) string {
	var he *proto.HandshakeError
	if errors.As(err, &he) {
		switch he.Code {
		case proto.ErrCodeUnauthorized:
			return "update ochq if you use the hosted server; for your own server, set OCHQ_TOKEN or pass --token"
		case proto.ErrCodeSubdomainTaken:
			return "pick another --sub, or omit it for a random one"
		case proto.ErrCodeVersionMismatch:
			return "update ochq to match the server"
		}
	}
	if errors.Is(err, tlsutil.ErrFingerprintMismatch) {
		return "if the server key was rotated on purpose, update OCHQ_FINGERPRINT; otherwise do not connect"
	}
	return ""
}

func listening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// displayAddr shows 127.0.0.1:3000 as localhost:3000, like the user typed it.
func displayAddr(addr string) string {
	if host, port, err := net.SplitHostPort(addr); err == nil && host == "127.0.0.1" {
		return "localhost:" + port
	}
	return addr
}

// parseLocalAddr accepts "3000" or "host:3000".
func parseLocalAddr(s string) (string, error) {
	host, portStr := "127.0.0.1", s
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, portStr = h, p
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 || strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("invalid port %q", s)
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}
