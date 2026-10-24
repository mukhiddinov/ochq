package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// glyphs are FIGlet "standard" letters, six rows each.
var glyphs = map[rune][bannerRows]string{
	'o': {"       ", "  ___  ", " / _ \\ ", "| (_) |", " \\___/ ", "       "},
	'c': {"      ", "  ___ ", " / __|", "| (__ ", " \\___|", "      "},
	'h': {" _     ", "| |__  ", "| '_ \\ ", "| | | |", "|_| |_|", "       "},
	'i': {" _ ", "(_)", "| |", "| |", "|_|", "   "},
	'q': {"       ", "  __ _ ", " / _` |", "| (_| |", " \\__, |", "    |_|"},
}

const (
	bannerRows = 6
	// The caret sits on the letters' baseline row, like a text cursor.
	caretRow = 4
	// caret is drawn as part of the banner: a terminal's own cursor is one
	// character cell, far smaller than these letters.
	caret   = "_____"
	tagline = "Open Channels, Handle Queries"
)

// rowColors fade from cyan to violet down the banner (xterm 256 colors).
var rowColors = [bannerRows]int{51, 45, 39, 33, 63, 99}

// render lays out a word the way FIGlet does: letters move together until
// they touch (kerning), then overlap one more column where the touching
// characters can merge (smushing), e.g. "|" over "|".
func render(word string) [bannerRows]string {
	var rows [bannerRows]string
	for i, r := range word {
		g := glyphs[r]
		if i == 0 {
			rows = g
			continue
		}
		rows = join(rows, g)
	}
	return rows
}

func join(a, b [bannerRows]string) [bannerRows]string {
	limit := min(len(a[0]), len(b[0]))
	k := 0
	for k < limit && overlaps(a, b, k+1, false) {
		k++
	}
	if k < limit && overlaps(a, b, k+1, true) {
		k++
	}
	var out [bannerRows]string
	for r := range bannerRows {
		left, right := a[r][:len(a[r])-k], b[r][k:]
		var mid strings.Builder
		for j := range k {
			c, _ := smush(a[r][len(a[r])-k+j], b[r][j])
			mid.WriteByte(c)
		}
		out[r] = left + mid.String() + right
	}
	return out
}

// overlaps reports whether the last k columns of a can share space with the
// first k of b: blanks always can, visible characters only if they smush.
func overlaps(a, b [bannerRows]string, k int, allowSmush bool) bool {
	for r := range bannerRows {
		for j := range k {
			x, y := a[r][len(a[r])-k+j], b[r][j]
			if x == ' ' || y == ' ' {
				continue
			}
			if _, ok := smush(x, y); !allowSmush || !ok {
				return false
			}
		}
	}
	return true
}

// smush applies FIGlet's equal-character and underscore rules.
func smush(x, y byte) (byte, bool) {
	const hierarchy = `|/\[]{}()<>`
	switch {
	case x == ' ':
		return y, true
	case y == ' ' || x == y:
		return x, true
	case x == '_' && strings.IndexByte(hierarchy, y) >= 0:
		return y, true
	case y == '_' && strings.IndexByte(hierarchy, x) >= 0:
		return x, true
	}
	return 0, false
}

// frame is one step of the typing animation.
type frame struct {
	word  string
	delay time.Duration // shown for this long before the next frame
}

// typing spells "ochiq" (Uzbek for "open"), takes back two letters and
// settles on the name: o, oc, och, ochi, ochiq, ochi, och, ochq. The caret
// follows the last letter all along.
var typing = []frame{
	{"o", 85 * time.Millisecond},
	{"oc", 85 * time.Millisecond},
	{"och", 85 * time.Millisecond},
	{"ochi", 85 * time.Millisecond},
	{"ochiq", 380 * time.Millisecond},
	{"ochi", 110 * time.Millisecond},
	{"och", 200 * time.Millisecond},
	{"ochq", 450 * time.Millisecond},
}

// blink is how the caret blinks after the name is typed: it goes off and on
// twice, then the version and tagline take its place.
var blink = struct{ on, off time.Duration }{on: 450 * time.Millisecond, off: 350 * time.Millisecond}

const blinkTimes = 2

// Terminal control sequences.
const (
	hideCursor        = "\033[?25l"
	showCursor        = "\033[?25h"
	clearLine         = "\r\033[2K"
	reserveRows       = "\n\n\n\n\n\n"
	cursorUpFmt       = "\033[%dA"
	ansi256Foreground = "\033[38;5;%dm"
)

func (p *Printer) bannerLines(word, version string, withCaret bool) [bannerRows]string {
	rows := render(word)
	if withCaret {
		rows[caretRow] = strings.TrimRight(rows[caretRow], " ") + " " + caret
	}
	width := len(render("ochq")[0])
	var out [bannerRows]string
	for r, line := range rows {
		line = fmt.Sprintf("%-*s", width, line)
		if p.color {
			line = fmt.Sprintf(ansi256Foreground, rowColors[r]) + bold + line + reset
		}
		out[r] = line
	}
	// The version and tagline belong to the finished logo only.
	if version != "" {
		out[4] += "   " + p.paint(bold, version)
		out[5] += "   " + p.paint(dim, tagline)
	}
	return out
}

// Banner prints the logo at once: for pipes, files and dumb terminals.
func (p *Printer) Banner(version string) {
	lines := p.bannerLines("ochq", version, false)
	p.printf("%s\n\n", strings.Join(lines[:], "\n"))
}

// AnimateBanner types the logo on a terminal and returns at once; anything
// else printed through p waits until the animation is done, so the tunnel
// connects meanwhile. Elsewhere it prints the static banner. Call Close
// before exiting to restore the cursor.
func (p *Printer) AnimateBanner(ctx context.Context, version string) {
	if !p.tty {
		p.Banner(version)
		return
	}
	p.mu.Lock()
	p.hidCursor = true
	go func() {
		defer p.mu.Unlock()
		p.typeBanner(ctx, version)
	}()
}

// typeBanner must run with p.mu held.
func (p *Printer) typeBanner(ctx context.Context, version string) {
	w := p.w
	fmt.Fprint(w, hideCursor+reserveRows)
	defer fmt.Fprint(w, "\n")
	// draw repaints the banner in the rows reserved above the cursor.
	draw := func(word, version string, withCaret bool) {
		fmt.Fprintf(w, cursorUpFmt, bannerRows)
		for _, l := range p.bannerLines(word, version, withCaret) {
			fmt.Fprint(w, clearLine+strings.TrimRight(l, " ")+"\n")
		}
	}
	for _, f := range typing {
		draw(f.word, "", true)
		if !sleep(ctx, f.delay) {
			return
		}
	}
	name := typing[len(typing)-1].word
	for range blinkTimes {
		draw(name, "", false)
		if !sleep(ctx, blink.off) {
			return
		}
		draw(name, "", true)
		if !sleep(ctx, blink.on) {
			return
		}
	}
	draw(name, version, false)
}

// sleep waits for d and reports false if ctx was cancelled first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// Close shows the terminal cursor again if AnimateBanner hid it. It waits
// for a running animation to finish.
func (p *Printer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.hidCursor {
		fmt.Fprint(p.w, showCursor)
		p.hidCursor = false
	}
}
