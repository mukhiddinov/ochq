package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRenderOchq(t *testing.T) {
	want := []string{
		"            _           ",
		"  ___   ___| |__   __ _ ",
		` / _ \ / __| '_ \ / _` + "`" + ` |`,
		"| (_) | (__| | | | (_| |",
		` \___/ \___|_| |_|\__, |`,
		"                     |_|",
	}
	got := render("ochq")
	for i := range want {
		if strings.TrimRight(got[i], " ") != strings.TrimRight(want[i], " ") {
			t.Fatalf("row %d:\n got %q\nwant %q", i, got[i], want[i])
		}
	}
}

func TestTypingSpellsOchiqThenOchq(t *testing.T) {
	var words []string
	for _, f := range typing {
		words = append(words, f.word)
	}
	if got := strings.Join(words, " "); got != "o oc och ochi ochiq ochi och ochq" {
		t.Fatalf("typing sequence: %s", got)
	}
	for _, f := range typing {
		for _, r := range f.word {
			if _, ok := glyphs[r]; !ok {
				t.Fatalf("no glyph for %q", r)
			}
		}
	}
}

// Not a terminal: one static banner, no escape sequences.
func TestStaticBannerOffTerminal(t *testing.T) {
	var buf bytes.Buffer
	p := New(&buf)
	p.AnimateBanner(context.Background(), "1.2.3")
	p.Close()
	out := buf.String()
	if strings.Contains(out, "\033") {
		t.Fatalf("escape sequences off a terminal: %q", out)
	}
	for _, want := range []string{"| (_) | (__| | | | (_| |", "1.2.3", tagline} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(_) __") {
		t.Error("static banner must not show the ochiq frame")
	}
}

func fastTyping(t *testing.T) {
	t.Helper()
	saved := typing
	typing = append([]frame(nil), typing...)
	for i := range typing {
		typing[i].delay = 0
	}
	savedBlink := blink
	blink.on, blink.off = 0, 0
	t.Cleanup(func() { typing, blink = saved, savedBlink })
}

func TestAnimationOnTerminal(t *testing.T) {
	fastTyping(t)
	var buf bytes.Buffer
	p := &Printer{w: &buf, tty: true, color: true, now: New(&buf).now}
	p.AnimateBanner(context.Background(), "1.2.3")
	p.Warn("after the banner") // must wait for the animation
	p.Close()

	out := buf.String()
	if !strings.HasPrefix(out, hideCursor) {
		t.Errorf("terminal cursor not hidden: %q", out[:20])
	}
	ochiq := strings.Index(out, "|/ _` |")   // "i" joined to "q": only in the ochiq frame
	final := strings.LastIndex(out, "1.2.3") // version appears with the last frame only
	warn := strings.Index(out, "after the banner")
	if ochiq < 0 || final < 0 || warn < 0 || !(ochiq < final && final < warn) {
		t.Fatalf("order ochiq=%d final=%d warn=%d", ochiq, final, warn)
	}
	if !strings.HasSuffix(out, showCursor) {
		t.Errorf("Close did not restore the cursor: %q", out[len(out)-20:])
	}
	// The caret follows every typed frame, then is drawn blinkTimes more
	// times while blinking; the final frame with the version has none.
	if n := strings.Count(out, caret); n != len(typing)+blinkTimes {
		t.Errorf("caret drawn %d times, want %d", n, len(typing)+blinkTimes)
	}
	finalFrame := out[strings.LastIndex(out, "\033[6A"):warn]
	if strings.Contains(finalFrame, caret) || !strings.Contains(finalFrame, "1.2.3") {
		t.Errorf("final frame should show the version instead of the caret: %q", finalFrame)
	}
	if n := strings.Count(out, tagline); n != 1 {
		t.Errorf("tagline drawn %d times; it belongs to the final frame only", n)
	}
}

func TestAnimationStopsOnCancel(t *testing.T) {
	var buf bytes.Buffer
	p := &Printer{w: &buf, tty: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.AnimateBanner(ctx, "1.2.3")
	p.Close() // returns only after the goroutine let go of the lock
	if strings.Contains(buf.String(), "ochq") && strings.Contains(buf.String(), "1.2.3") {
		t.Error("a cancelled animation should not reach the final frame")
	}
	if !strings.HasSuffix(buf.String(), showCursor) {
		t.Error("cursor not restored after cancel")
	}
}
