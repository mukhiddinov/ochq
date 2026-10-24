package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mukhiddinov/ochq/internal/client"
	"github.com/mukhiddinov/ochq/pkg/version"
)

// execute runs the CLI like main does, with a fake tunnel that records the
// config it was given instead of dialing anything.
func execute(t *testing.T, args ...string) (cfg *client.Config, out string, err error) {
	t.Helper()
	var buf bytes.Buffer
	fake := func(_ context.Context, c client.Config) error {
		cfg = &c
		return nil
	}
	cmd := newRootCmd(fake, &buf)
	cmd.SetArgs(compatArgs(args))
	cmd.SetErr(&buf)
	err = cmd.ExecuteContext(context.Background())
	return cfg, buf.String(), err
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"OCHQ_SERVER", "OCHQ_TOKEN", "OCHQ_FINGERPRINT"} {
		t.Setenv(k, "")
	}
}

// Every way of starting a tunnel that ochq 0.1 accepted must still work.
func TestTunnelInvocations(t *testing.T) {
	clearEnv(t)
	tests := []struct {
		args   []string
		local  string
		sub    string
		server string
		token  string
	}{
		{args: []string{"3000"}, local: "127.0.0.1:3000", server: defaultServer, token: defaultToken},
		{args: []string{"http", "8080"}, local: "127.0.0.1:8080", server: defaultServer, token: defaultToken},
		{args: []string{"3000", "--sub", "My-API"}, local: "127.0.0.1:3000", sub: "my-api", server: defaultServer, token: defaultToken},
		{args: []string{"--sub", "demo", "3000"}, local: "127.0.0.1:3000", sub: "demo", server: defaultServer, token: defaultToken},
		{args: []string{"3000", "--sub", "Demo.ochq.uz"}, local: "127.0.0.1:3000", sub: "demo", server: defaultServer, token: defaultToken},
		{args: []string{"http", "3000", "--sub", "demo", "--server", "tunnel.example.com:9000"}, local: "127.0.0.1:3000", sub: "demo", server: "tunnel.example.com:9000"},
		{args: []string{"3000", "--sub", "demo.example.com", "--server", "tunnel.example.com:9000"}, local: "127.0.0.1:3000", sub: "demo", server: "tunnel.example.com:9000"},
		{args: []string{"192.168.1.20:80"}, local: "192.168.1.20:80", server: defaultServer, token: defaultToken},
		// ochq 0.1 used Go's flag package: single-dash long flags.
		{args: []string{"3000", "-sub", "demo", "-server=a.b:1", "-token", "t"}, local: "127.0.0.1:3000", sub: "demo", server: "a.b:1", token: "t"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			cfg, out, err := execute(t, tt.args...)
			if err != nil {
				t.Fatalf("error: %v\n%s", err, out)
			}
			if cfg == nil {
				t.Fatal("tunnel was not started")
			}
			if cfg.LocalAddr != tt.local || cfg.Subdomain != tt.sub || cfg.ServerAddr != tt.server || cfg.Token != tt.token {
				t.Fatalf("got %+v", *cfg)
			}
		})
	}
}

// The built-in token and fingerprint belong to the hosted server only; they
// must never be sent to, or used to verify, a server the user picked.
func TestBuiltInCredentials(t *testing.T) {
	clearEnv(t)
	cfg, _, err := execute(t, "3000")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != defaultToken || cfg.Fingerprint != defaultFingerprint {
		t.Fatalf("hosted server needs no setup, got %+v", *cfg)
	}

	cfg, _, err = execute(t, "3000", "--server", "tunnel.example.com:9000")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" || cfg.Fingerprint != "" {
		t.Fatalf("built-in credentials leaked to another server: %+v", *cfg)
	}

	cfg, _, err = execute(t, "3000", "--token", "mine", "--fingerprint", "SHA256:mine")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "mine" || cfg.Fingerprint != "SHA256:mine" {
		t.Fatalf("flags must win over built-in values: %+v", *cfg)
	}
}

func TestEnvironmentAndPrecedence(t *testing.T) {
	t.Setenv("OCHQ_SERVER", "env.example:9000")
	t.Setenv("OCHQ_TOKEN", "env-token")
	t.Setenv("OCHQ_FINGERPRINT", "SHA256:env")

	cfg, _, err := execute(t, "3000")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddr != "env.example:9000" || cfg.Token != "env-token" || cfg.Fingerprint != "SHA256:env" {
		t.Fatalf("environment not applied: %+v", *cfg)
	}

	cfg, _, err = execute(t, "3000", "--server", "flag.example:9000", "--token", "flag-token")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerAddr != "flag.example:9000" || cfg.Token != "flag-token" || cfg.Fingerprint != "SHA256:env" {
		t.Fatalf("flags must win over the environment: %+v", *cfg)
	}
}

// Help prints flag defaults; a token from the environment must not be one.
func TestHelpDoesNotLeakSecrets(t *testing.T) {
	t.Setenv("OCHQ_TOKEN", "super-secret-token")
	t.Setenv("OCHQ_FINGERPRINT", "SHA256:secretpin")
	for _, args := range [][]string{{"--help"}, {"help"}, {"help", "http"}, {"-help"}, {}} {
		_, out, err := execute(t, args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Contains(out, "super-secret-token") || strings.Contains(out, "secretpin") {
			t.Fatalf("%v leaks a secret:\n%s", args, out)
		}
		if !strings.Contains(out, "Usage:") {
			t.Fatalf("%v: no usage in output:\n%s", args, out)
		}
	}
}

func TestHelpContent(t *testing.T) {
	_, out, _ := execute(t, "help")
	for _, want := range []string{"Examples:", "ochq 3000 --sub my-api", "OCHQ_TOKEN", "http", "version", "completion"} {
		if !strings.Contains(out, want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

func TestVersion(t *testing.T) {
	want := "ochq " + version.Version + "\n"
	for _, args := range [][]string{{"version"}, {"--version"}, {"-v"}, {"-version"}} {
		cfg, out, err := execute(t, args...)
		if err != nil || out != want || cfg != nil {
			t.Errorf("%v: got %q, %v (tunnel started: %v)", args, out, err, cfg != nil)
		}
	}
}

func TestErrors(t *testing.T) {
	clearEnv(t)
	tests := map[string][]string{
		`invalid port "abc"`:            {"abc"},
		`invalid port "99999"`:          {"99999"},
		`invalid --sub "a_b"`:           {"3000", "--sub", "a_b"},
		`invalid --sub "demo.other.uz"`: {"3000", "--sub", "demo.other.uz"},
		`unknown command "verison"`:     {"verison"},
		`did you mean "version"`:        {"verison"},
		"accepts 1 arg(s), received 0":  {"http"},
		"unknown flag: --sevrer":        {"3000", "--sevrer", "x"},
	}
	for want, args := range tests {
		cfg, _, err := execute(t, args...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%v: got %v, want error containing %q", args, err, want)
		}
		if cfg != nil {
			t.Errorf("%v: tunnel started despite the error", args)
		}
	}
}

func TestCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		_, out, err := execute(t, "completion", shell)
		if err != nil || !strings.Contains(out, "ochq") || len(out) < 500 {
			t.Errorf("completion %s: err=%v, %d bytes", shell, err, len(out))
		}
	}
}

func TestCompatArgsLeavesOtherArgsAlone(t *testing.T) {
	in := []string{"3000", "-h", "--sub", "x", "-sub=y", "-subdomain", "-", "-version"}
	got := strings.Join(compatArgs(in), " ")
	want := "3000 -h --sub x --sub=y -subdomain - --version"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
