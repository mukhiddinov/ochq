package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func testPrinter() (*Printer, *bytes.Buffer) {
	var buf bytes.Buffer
	p := New(&buf)
	p.now = func() time.Time { return time.Date(2026, 10, 23, 14, 2, 7, 0, time.UTC) }
	return p, &buf
}

func TestNoColorWhenNotATerminal(t *testing.T) {
	p, buf := testPrinter()
	p.Online("https://x8k2.ochq.uz", "http://localhost:3000")
	if strings.Contains(buf.String(), "\033[") {
		t.Fatalf("escape codes written to a non-terminal: %q", buf.String())
	}
}

func TestOnline(t *testing.T) {
	p, buf := testPrinter()
	p.Online("https://x8k2.ochq.uz", "http://localhost:3000")
	for _, want := range []string{
		"[✓] Status:       Online",
		"[✓] Forwarding:   http://localhost:3000",
		"[✓] Public URL:   https://x8k2.ochq.uz",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestRequestLine(t *testing.T) {
	p, buf := testPrinter()
	p.Request("GET", "/api/v1/health", 200, 14*time.Millisecond)
	line := buf.String()
	for _, want := range []string{"14:02:07", "GET", "/api/v1/health", "200 OK", "14ms"} {
		if !strings.Contains(line, want) {
			t.Errorf("missing %q in %q", want, line)
		}
	}
}

func TestLongPathTruncated(t *testing.T) {
	p, buf := testPrinter()
	p.Request("GET", "/"+strings.Repeat("a", 200), 404, time.Second)
	if strings.Contains(buf.String(), strings.Repeat("a", maxPathWidth)) || !strings.Contains(buf.String(), "…") {
		t.Fatalf("path not truncated: %q", buf.String())
	}
}

func TestReconnecting(t *testing.T) {
	p, buf := testPrinter()
	p.Reconnecting(3, 4*time.Second, errors.New("connection refused"))
	for _, want := range []string{"Reconnecting", "connection refused", "4s", "attempt 3"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in %q", want, buf.String())
		}
	}
}

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		250 * time.Microsecond:  "250µs",
		42 * time.Millisecond:   "42ms",
		1234 * time.Millisecond: "1.23s",
		5 * time.Minute:         "5m0s",
	} {
		if got := formatDuration(d); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestLogsHeadingOnce(t *testing.T) {
	p, buf := testPrinter()
	p.Online("https://a.ochq.uz", "http://localhost:3000")
	p.Online("https://a.ochq.uz", "http://localhost:3000") // after a reconnect
	if n := strings.Count(buf.String(), "Logs (Ctrl+C to stop):"); n != 1 {
		t.Fatalf("Logs heading printed %d times", n)
	}
}
