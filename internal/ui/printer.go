// Package ui renders the ochq terminal output: banner, tunnel status and
// the live request log. Colors are used only on a terminal and can be
// turned off with NO_COLOR (https://no-color.org).
package ui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

const banner = `              _
  ___   ___  | |__    __ _
 / _ \ / __| | '_ \  / _` + "`" + ` |
| (_) | (__  | | | || (_| |
 \___/ \___| |_| |_| \__, |   %s
                        |_|   Open Channels, Handle Queries
`

const maxPathWidth = 48

// ANSI SGR codes.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

// Printer is safe for concurrent use: requests finish on many goroutines.
type Printer struct {
	mu        sync.Mutex
	w         io.Writer
	color     bool
	now       func() time.Time
	shownLogs bool // the "Logs" heading is printed once, not after every reconnect
}

// New writes to w, with colors if w is a terminal and NO_COLOR is unset.
func New(w io.Writer) *Printer {
	return &Printer{w: w, color: isTerminal(w) && os.Getenv("NO_COLOR") == "", now: time.Now}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (p *Printer) paint(code, s string) string {
	if !p.color {
		return s
	}
	return code + s + reset
}

func (p *Printer) printf(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fmt.Fprintf(p.w, format, args...)
}

func (p *Printer) Banner(version string) {
	p.printf("%s\n", p.paint(cyan, fmt.Sprintf(banner, version)))
}

func (p *Printer) Connecting(server string) {
	p.printf("%s Connecting to %s ...\n", p.paint(dim, "[…]"), server)
}

func (p *Printer) Online(publicURL, localURL string) {
	ok := p.paint(green, "[✓]")
	p.printf("%s Status:       %s\n%s Forwarding:   %s\n%s Public URL:   %s\n",
		ok, p.paint(green+bold, "Online"),
		ok, localURL,
		ok, p.paint(bold, publicURL))

	p.mu.Lock()
	first := !p.shownLogs
	p.shownLogs = true
	p.mu.Unlock()
	if first {
		p.printf("\n%s\n", p.paint(dim, "Logs (Ctrl+C to stop):"))
	}
}

func (p *Printer) Reconnecting(attempt int, delay time.Duration, err error) {
	p.printf("%s Status:       %s — %v\n%s retrying in %s (attempt %d)\n",
		p.paint(yellow, "[!]"), p.paint(yellow+bold, "Reconnecting"), err,
		p.paint(dim, "   "), delay.Round(100*time.Millisecond), attempt)
}

func (p *Printer) Warn(msg string) {
	p.printf("%s %s\n", p.paint(yellow, "[!]"), msg)
}

// Request prints one log line, e.g.
//
//	14:02:07  GET   /api/v1/health   200 OK   14ms
func (p *Printer) Request(method, path string, status int, d time.Duration) {
	if len(path) > maxPathWidth {
		path = path[:maxPathWidth-1] + "…"
	}
	code := fmt.Sprintf("%d %s", status, http.StatusText(status))
	p.printf("%s  %-6s %-*s  %s  %s\n",
		p.paint(dim, p.now().Format("15:04:05")),
		method, maxPathWidth, path,
		p.paint(statusColor(status), fmt.Sprintf("%-24s", code)),
		p.paint(dim, formatDuration(d)))
}

func statusColor(status int) string {
	switch {
	case status >= 500:
		return red
	case status >= 400:
		return yellow
	case status >= 300:
		return cyan
	default:
		return green
	}
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return d.Round(10 * time.Millisecond).String()
	}
}
