package web

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mukhiddinov/ochq/internal/ui"
	site "github.com/mukhiddinov/ochq/web"
)

var update = flag.Bool("update", false, "rewrite generated files")

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

// The install demo replays the real banner from banner.json; it must never
// drift from internal/ui. Run `go test ./internal/web -update` after
// changing the banner.
func TestBannerJSONInSync(t *testing.T) {
	want, err := json.MarshalIndent(bannerData(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	const path = "../../web/site/assets/banner.json"
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("web/site/assets/banner.json is stale: run go test ./internal/web -update")
	}
	if n := len(bannerData().Frames); n != len(ui.AnimationFrames()) {
		t.Fatalf("%d frames", n)
	}
}

func TestPageUz(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/")
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("GET / = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	for _, want := range []string{
		`<html lang="uz">`,
		`<link rel="canonical" href="https://ochq.uz/">`,
		`hreflang="x-default"`,
		`id="hero"`, `id="how"`, `id="install"`, `id="usage"`, `id="about"`, `id="comments"`,
		`brew install --cask mukhiddinov/tap/ochq`,
		`https://www.linkedin.com/in/khusniddin-mukhiddinov`,
		`https://github.com/mukhiddinov`,
		`Middle Software Engineer`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Contains(body, "snap install") {
		t.Error("page mentions snap before the Snap Store approved the name")
	}
}

// Every command on the page must also be in the README, so the two never
// tell people different things.
func TestInstallCommandsMatchReadme(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	page := get(t, newTestServer(t), "/").Body.String()
	for _, cmd := range []string{
		"brew install --cask mukhiddinov/tap/ochq",
		"curl -fsSL https://mukhiddinov.github.io/ochq/install.sh | sh",
		"sudo dnf copr enable mukhiddinov/ochq",
		"sudo dnf install ochq",
		"scoop bucket add ochq https://github.com/mukhiddinov/scoop-bucket",
		"scoop install ochq",
	} {
		if !strings.Contains(string(readme), cmd) {
			t.Errorf("README lacks %q", cmd)
		}
		if !strings.Contains(page, htmlEscape(cmd)) {
			t.Errorf("page lacks %q", cmd)
		}
	}
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func TestAssets(t *testing.T) {
	s := newTestServer(t)
	w := get(t, s, "/assets/site.css")
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/css") || !strings.Contains(w.Header().Get("Cache-Control"), "max-age") {
		t.Fatalf("site.css: %d %v", w.Code, w.Header())
	}
	w = get(t, s, "/install.sh")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "https://mukhiddinov.github.io/ochq/install.sh" {
		t.Fatalf("install.sh: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w = get(t, s, "/nope"); w.Code != 404 {
		t.Fatalf("/nope: %d", w.Code)
	}
	if w = get(t, s, "/page.html.tmpl"); w.Code != 404 {
		t.Fatalf("template is served: %d", w.Code)
	}
}

func TestPageWeight(t *testing.T) {
	total := len(get(t, newTestServer(t), "/").Body.Bytes())
	for _, f := range []string{"site/assets/site.css", "site/assets/site.js", "site/assets/banner.json"} {
		b, err := fs.ReadFile(site.FS, f)
		if err != nil {
			t.Fatal(err)
		}
		total += len(b)
	}
	if total >= 100_000 {
		t.Fatalf("page weighs %d bytes, budget 100000", total)
	}
}
