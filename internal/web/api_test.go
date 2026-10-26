package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

const proxy = "172.30.0.2"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(Config{
		DBPath:       filepath.Join(t.TempDir(), "comments.jsonl"),
		Salt:         "test-salt",
		TrustedProxy: proxy,
		Now:          newClock().now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// post sends body as JSON from the visitor ip, through the trusted proxy.
func post(t *testing.T, s *Server, ip, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/api/comments", strings.NewReader(body))
	r.RemoteAddr = proxy + ":40000"
	r.Header.Set("X-Forwarded-For", ip)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func list(t *testing.T, s *Server, query string) []Comment {
	t.Helper()
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/comments"+query, nil))
	if w.Code != 200 {
		t.Fatalf("GET status %d", w.Code)
	}
	var out []Comment
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET body %q: %v", w.Body, err)
	}
	return out
}

func TestPostComment(t *testing.T) {
	s := newTestServer(t)
	w := post(t, s, "1.1.1.1", `{"name":"Ali","body":"salom","lang":"uz"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d %s", w.Code, w.Body)
	}
	var got map[string]any
	json.Unmarshal(w.Body.Bytes(), &got)
	for _, k := range []string{"id", "name", "body", "lang", "created_at"} {
		if _, ok := got[k]; !ok {
			t.Errorf("response lacks %q: %s", k, w.Body)
		}
	}
	if cs := list(t, s, ""); len(cs) != 1 || cs[0].Name != "Ali" || cs[0].Body != "salom" {
		t.Fatalf("list = %+v", cs)
	}
}

func TestPostValidation(t *testing.T) {
	tests := []struct {
		name, body string
		want       int
	}{
		{"empty body", `{"body":"","lang":"uz"}`, 400},
		{"blank body", `{"body":"   ","lang":"uz"}`, 400},
		{"long body", `{"body":"` + strings.Repeat("a", 1001) + `","lang":"uz"}`, 400},
		{"long name", `{"name":"` + strings.Repeat("a", 41) + `","body":"x","lang":"uz"}`, 400},
		{"bad lang", `{"body":"x","lang":"de"}`, 400},
		{"control char", `{"body":"a\u0000b","lang":"uz"}`, 400},
		{"invalid json", `{"body":`, 400},
		{"1000 runes", `{"body":"` + strings.Repeat("ў", 1000) + `","lang":"ru"}`, 201},
		{"newlines ok", `{"body":"a\nb\tc","lang":"en"}`, 201},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			if w := post(t, s, fmt.Sprintf("2.2.2.%d", i), tt.body); w.Code != tt.want {
				t.Fatalf("status %d, want %d (%s)", w.Code, tt.want, w.Body)
			}
		})
	}
}

func TestPostHoneypot(t *testing.T) {
	s := newTestServer(t)
	if w := post(t, s, "1.1.1.1", `{"body":"buy now","lang":"en","website":"x"}`); w.Code != http.StatusCreated {
		t.Fatalf("honeypot status %d, want a fake 201", w.Code)
	}
	if cs := list(t, s, ""); len(cs) != 0 {
		t.Fatalf("honeypot comment stored: %+v", cs)
	}
}

func TestPostRateLimit(t *testing.T) {
	s := newTestServer(t)
	for i := range 3 {
		if w := post(t, s, "3.3.3.3", `{"body":"hi","lang":"uz"}`); w.Code != 201 {
			t.Fatalf("post %d status %d", i+1, w.Code)
		}
	}
	if w := post(t, s, "3.3.3.3", `{"body":"hi","lang":"uz"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th post status %d, want 429", w.Code)
	}
	// Straight to the server (not via Traefik) with a made-up X-Forwarded-For:
	// the header is ignored, so this is still a fresh address, 5.5.5.5.
	for i := range 4 {
		r := httptest.NewRequest("POST", "/api/comments", strings.NewReader(`{"body":"hi","lang":"uz"}`))
		r.RemoteAddr = "5.5.5.5:1234"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if want := map[bool]int{true: 201, false: 429}[i < 3]; w.Code != want {
			t.Fatalf("spoofed post %d status %d, want %d", i+1, w.Code, want)
		}
	}
}

func TestPostTooLarge(t *testing.T) {
	s := newTestServer(t)
	body := `{"body":"` + strings.Repeat("a", 9<<10) + `","lang":"uz"}`
	if w := post(t, s, "1.1.1.1", body); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", w.Code)
	}
}

func TestHTMLStaysText(t *testing.T) {
	s := newTestServer(t)
	post(t, s, "1.1.1.1", `{"body":"<script>alert(1)</script>","lang":"uz"}`)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/api/comments", nil))
	b, _ := io.ReadAll(w.Body)
	if strings.Contains(string(b), "<script>") || !strings.Contains(string(b), `\u003cscript\u003e`) {
		t.Fatalf("body not escaped: %s", b)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type %q", ct)
	}
}

func TestGetPaging(t *testing.T) {
	s := newTestServer(t)
	for i := range 25 {
		s.store.Add(Comment{Body: fmt.Sprint(i), Lang: "uz"}, "")
	}
	first := list(t, s, "")
	if len(first) != 20 {
		t.Fatalf("first page %d, want 20", len(first))
	}
	rest := list(t, s, fmt.Sprintf("?before=%d", first[19].ID))
	if len(rest) != 5 {
		t.Fatalf("second page %d, want 5", len(rest))
	}
	if all := list(t, s, "?limit=500"); len(all) != 25 {
		t.Fatalf("limit=500 gave %d", len(all))
	}
	for i := range 30 {
		s.store.Add(Comment{Body: fmt.Sprint(i), Lang: "uz"}, "")
	}
	if capped := list(t, s, "?limit=500"); len(capped) != 50 {
		t.Fatalf("limit cap: got %d, want 50", len(capped))
	}
}

func TestHealthz(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/healthz", nil))
	if w.Code != 200 || w.Body.String() != "ok\n" {
		t.Fatalf("healthz %d %q", w.Code, w.Body)
	}
}
