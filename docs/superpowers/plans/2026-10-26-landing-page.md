# ochq.uz Landing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Serve a three-language landing page with animations and anonymous-friendly comments at `https://ochq.uz`, and stop tunnels from claiming reserved names.

**Architecture:** A new binary `ochq-web` (std `net/http`) renders one `html/template` page per language from embedded i18n JSON, serves embedded static assets, and exposes a small comments API backed by an append-only JSON-lines log. It runs on the VPS as its own systemd service behind the existing Traefik edge; CI deploys it separately from `ochqd`. Front-end is vanilla JS/CSS reusing the approved mockups.

**Tech Stack:** Go 1.26 stdlib only (`net/http`, `html/template`, `embed`, `encoding/json`); vanilla HTML/CSS/JS; systemd; GitHub Actions; Traefik file provider (repo `vps-edge`).

**Spec:** `docs/superpowers/specs/2026-10-26-landing-page-design.md`

**Approved mockups (copy look + animation logic from these):**
`.superpowers/brainstorm/16728-1793032452/content/hero.html` (section `#a` only),
`.../how-it-works.html`, `.../install-terminal.html`.

## Deviation from spec (Claude's call, tell the user)

Storage is an append-only JSON-lines file, not SQLite. Reason: COPR builds
vendor every module in `go.mod`; `modernc.org/sqlite` + its libc would add
tens of MB to every SRPM. Comment volume is tiny; a log file is enough. The
spec is updated in Task 2.

## Global Constraints

- Everything on the VPS is named `ochq*`: service user `ochq_web`, deploy user `ochq_web_deploy`, unit `ochq-web`, `/opt/ochq-web`, `/etc/ochq/ochq-web.env`, state `/var/lib/ochq-web`.
- Never touch efbazaar, examiner, mailcow or the `ochqd` deploy path (`deploy/ochq-deploy`, user `ochq_deploy`).
- No new Go module dependencies.
- Comments: name 0–40 runes, body 1–1000 runes, empty name shown as localised "ghost"; rendered with `textContent` only.
- Rate limits: 3 comments / 10 min per client IP; 60 / hour global. Request body ≤ 8 KB.
- Client IP from `X-Forwarded-For` (rightmost entry) only when `RemoteAddr` is the trusted proxy `172.30.0.2`; stored only as `hex(sha256(salt+ip))[:16]`.
- URLs: `/` = uz (x-default), `/en/`, `/ru/`. No auto-redirect by language.
- Palette/typography exactly as in the spec "Approved visual direction".
- Page weight (HTML+CSS+JS, no fonts) < 100 KB uncompressed.
- Snap is not mentioned anywhere on the page.
- Commits and PRs carry NO Co-Authored-By / Claude-Session / "Generated with" lines.
- One PR per delivery step; branch per PR from up-to-date `main`; the user merges.

## Review Focus

1. A tunnel requesting `WWW`, ` demo `, or `Api` (case/space variants) must be refused like the lowercase name — test in Task 1.
2. A comment body with `<script>`/`<img onerror>` must appear as literal text on the page — test in Task 2 (API stores raw, JSON-escapes) and Task 5 (JS uses `textContent`; grep check).
3. A spoofed `X-Forwarded-For` from a non-proxy address must not bypass the rate limit — test in Task 2.
4. Moderation run while the server is up (`ochq-web hide 3`) must take effect without restart — test in Task 2.
5. Visitors with `prefers-reduced-motion` or on a phone must get a static/light page with no horizontal scroll — manual check in Task 6, listed in its verification step.

---

## PR 1 — Reserved subdomains (branch `feat/reserved-subdomains`)

### Task 1: Refuse reserved names in the handshake

**Files:**
- Create: `internal/server/reserved.go`
- Modify: `internal/server/server.go:197-203` (after `NormalizeSubdomain`)
- Modify: `internal/server/registry.go` (`ReserveRandom` skips reserved)
- Test: `internal/server/server_test.go`, rename `"demo"` → `"myapp"` in `internal/client/runner_test.go:94,97,150,152` and in `TestHandshakeRejects` "taken" case

**Interfaces:**
- Produces: `func isReserved(sub string) bool` (expects normalised input).

- [ ] **Step 1: Write failing tests** — add cases to `TestHandshakeRejects`:

```go
{"reserved", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: "www"}, proto.ErrCodeInvalidSubdomain},
{"reserved case", proto.Hello{Version: proto.Version, Type: proto.TunnelHTTP, Token: "s3cret", Subdomain: " Demo "}, proto.ErrCodeInvalidSubdomain},
```

and change the existing "taken" case + its `s.registry.Reserve(...)` to `"myapp"`. Add:

```go
func TestIsReserved(t *testing.T) {
	for _, s := range []string{"www", "api", "admin", "mail", "login", "status", "docs", "blog", "app", "dashboard", "ochq", "tunnel", "pkg", "demo"} {
		if !isReserved(s) { t.Errorf("%q should be reserved", s) }
	}
	for _, s := range []string{"myapp", "a7f3", "demo2", "wwww"} {
		if isReserved(s) { t.Errorf("%q should not be reserved", s) }
	}
}
```

- [ ] **Step 2: Run** `go test ./internal/server/ -run 'TestIsReserved|TestHandshakeRejects'` — Expected: FAIL (`isReserved` undefined).
- [ ] **Step 3: Implement** `isReserved` as a `map[string]struct{}` of exactly the 14 names above, with a comment saying why (`demo` appears on ochq.uz; `tunnel` is the control endpoint; `pkg` is planned). In `handshake`, after normalising: `if isReserved(sub) { return reject(proto.ErrCodeInvalidSubdomain, "subdomain %q is reserved", sub) }`. In `ReserveRandom`, `continue` when `isReserved(sub)`.
- [ ] **Step 4: Run** `gofmt -l . && go vet ./... && go test -race ./...` — Expected: no gofmt output, all PASS.
- [ ] **Step 5: Commit** `feat(server): refuse reserved subdomains` and open PR "Refuse reserved subdomains" (body: what/why, test list). Wait for the user to merge.

---

## PR 2 — ochq-web service (branch `feat/ochq-web`)

### Task 2: Comment store, limiter, client IP, API, CLI

**Files:**
- Create: `internal/web/store.go`, `internal/web/limit.go`, `internal/web/api.go`, `internal/web/server.go`
- Create: `cmd/ochq-web/main.go`
- Test: `internal/web/store_test.go`, `internal/web/limit_test.go`, `internal/web/api_test.go`
- (Spec already records the JSON-lines decision.)

**Interfaces:**
- Produces:
  ```go
  type Comment struct {
  	ID        int64     `json:"id"`
  	Name      string    `json:"name"`
  	Body      string    `json:"body"`
  	Lang      string    `json:"lang"`
  	CreatedAt time.Time `json:"created_at"`
  	Hidden    bool      `json:"-"`
  }
  func OpenStore(path string) (*Store, error)
  func (s *Store) Add(c Comment, ipHash string) (Comment, error) // assigns ID, CreatedAt
  func (s *Store) List(before int64, limit int, withHidden bool) ([]Comment, error) // newest first; before<=0 means from newest
  func (s *Store) SetHidden(id int64, hidden bool) error // ErrNotFound if id unknown

  func NewLimiter(perKey int, perKeyWindow time.Duration, global int, globalWindow time.Duration, now func() time.Time) *Limiter
  func (l *Limiter) Allow(key string) bool

  func ClientIP(r *http.Request, trustedProxy string) string

  type Config struct { DBPath, Salt, TrustedProxy string; Now func() time.Time }
  func NewServer(cfg Config) (*Server, error) // *Server implements http.Handler
  ```
- Log format, one JSON object per line: `{"op":"add","id":N,"name":…,"body":…,"lang":…,"created_at":…,"ip":"<hash>"}` and `{"op":"hide","id":N}` / `{"op":"unhide","id":N}`. Writers open with `O_APPEND|O_CREATE|O_WRONLY`, write one line in a single `Write`, `Sync`. Readers keep the byte offset and, before every `List`/`Add`, read only new bytes (so a CLI `hide` appended by another process is seen without restart). IDs = max seen + 1, guarded by a mutex; a torn last line (no `\n`) is left for the next read.

- [ ] **Step 1: Write failing store tests** (`store_test.go`, temp dir):
  - `TestStoreAddList`: add 3 → `List(0,2,false)` returns IDs `[3,2]`; `List(2,10,false)` returns `[1]`.
  - `TestStoreHideVisibleAcrossProcesses`: open store A and store B on the same file; A adds 2 comments; B `SetHidden(1,true)`; A `List(0,10,false)` returns only ID 2; `withHidden=true` returns both.
  - `TestStoreReopen`: add, close, `OpenStore` again → IDs continue at 2, hidden state preserved.
  - `TestStoreSetHiddenUnknown`: `SetHidden(42,true)` returns `ErrNotFound`.
- [ ] **Step 2: Run** `go test ./internal/web/ -run TestStore` — Expected: FAIL (package missing).
- [ ] **Step 3: Implement `store.go`** per the interface and log format above.
- [ ] **Step 4: Run** same command — Expected: PASS.
- [ ] **Step 5: Write failing limiter + IP tests:**
  - `TestLimiterPerKey`: with fake clock, key "a" allowed 3×, 4th denied; after +10m1s allowed again; key "b" unaffected.
  - `TestLimiterGlobal`: global=2 → third distinct key denied.
  - `TestClientIP`: RemoteAddr `172.30.0.2:5000` + XFF `"1.1.1.1, 9.9.9.9"` → `"9.9.9.9"`; RemoteAddr `5.5.5.5:1` + XFF `"9.9.9.9"` → `"5.5.5.5"` (spoof ignored); trusted proxy without XFF → `"172.30.0.2"`.
- [ ] **Step 6: Run** `go test ./internal/web/ -run 'TestLimiter|TestClientIP'` — FAIL; **implement** `limit.go` (sliding window of timestamps per key; prune on each call) and `ClientIP` in `api.go`; re-run — PASS.
- [ ] **Step 7: Write failing API tests** (`api_test.go`, `httptest`, fake clock):
  - `TestPostComment`: POST `{"name":"Ali","body":"salom","lang":"uz"}` → 201, JSON has `id`,`name`,`body`,`lang`,`created_at`; then GET `/api/comments` returns it.
  - `TestPostValidation` table → 400 for: body `""`, body `"   "`, body 1001×"a", name 41×"a", lang `"de"`, body with `"\x00"`, invalid JSON; body exactly 1000 runes of `"ў"` → 201.
  - `TestPostHoneypot`: `"website":"x"` → 201 but GET returns nothing.
  - `TestPostRateLimit`: 3 posts from one IP → 201, 4th → 429; spoofed XFF from non-proxy RemoteAddr does not reset the count.
  - `TestPostTooLarge`: 9 KB body → 413.
  - `TestHTMLStaysText`: post body `<script>alert(1)</script>`; GET response JSON contains `<script>` (Go's default escaping) and never the raw `<script>`.
  - `TestGetPaging`: 25 comments → GET returns 20, `?before=<last id>` returns 5; `limit` capped at 50.
- [ ] **Step 8: Implement** `api.go` handlers + `server.go` (`NewServer` builds a mux: `GET /api/comments`, `POST /api/comments`, `GET /healthz` → `200 ok`). Trim name/body with `strings.TrimSpace`, count runes with `utf8.RuneCountInString`, reject runes `< 0x20` except `\n`, `\t`. Use `http.MaxBytesReader(w, r.Body, 8<<10)`. Limiter key = ip hash.
- [ ] **Step 9: Run** `go test -race ./internal/web/` — PASS.
- [ ] **Step 10: Implement `cmd/ochq-web/main.go`** (std `flag`, mirroring `cmd/ochqd/main.go` style): subcommand from `os.Args[1]`:
  - none / `serve`: flags `-addr` [$OCHQ_WEB_ADDR, default `127.0.0.1:8090`], `-db` [$OCHQ_WEB_DB, default `/var/lib/ochq-web/comments.jsonl`], `-trusted-proxy` [$OCHQ_WEB_TRUSTED_PROXY, default `172.30.0.2`]; salt from $OCHQ_WEB_SALT, required (exit 2 with a message if empty); `http.Server` with `ReadHeaderTimeout: 5s`; graceful shutdown on SIGINT/SIGTERM.
  - `list [-n 20]`: prints `id  created_at  lang  hidden  name: body` incl. hidden.
  - `hide <id>` / `unhide <id>`.
  - `-version` prints `ochq-web <version>` using `pkg/version.Version`.
- [ ] **Step 11: Smoke-run locally:** `OCHQ_WEB_SALT=x go run ./cmd/ochq-web -db /tmp/c.jsonl &`, `curl -s -XPOST localhost:8090/api/comments -d '{"body":"hi","lang":"en"}'` → 201; `go run ./cmd/ochq-web list -db /tmp/c.jsonl` shows it; `hide 1` → GET shows `[]`. Kill server.
- [ ] **Step 12: Run** `gofmt -l . && go vet ./... && go test -race ./...` — clean.
- [ ] **Step 13: Commit** `feat(web): add ochq-web with the comments API`; PR "ochq-web: comments API"; wait for merge.

---

## PR 3 — Uzbek landing page (branch `feat/landing-uz`)

### Task 3: Page rendering, i18n, static assets, banner frames

**Files:**
- Create: `web/web.go` (`package web`; `//go:embed site i18n` → `var FS embed.FS`)
- Create: `web/site/page.html.tmpl`, `web/site/assets/site.css`, `web/site/assets/site.js`, `web/site/assets/banner.json`, `web/site/assets/og.png` (1200×630, dark, logo), `web/site/favicon.svg`
- Create: `web/i18n/uz.json`
- Create: `internal/web/pages.go`
- Modify: `internal/ui/banner.go` (export frames)
- Test: `internal/web/pages_test.go`, `internal/ui/banner_test.go`

**Interfaces:**
- Consumes: `NewServer` mux from Task 2.
- Produces:
  ```go
  // internal/ui
  type Frame struct { Rows [6]string `json:"rows"`; DelayMs int `json:"delay_ms"` }
  func AnimationFrames() []Frame   // typing frames (with caret) + 2×(off,on) blinks; no colour codes
  var RowHex = [6]string{"#00ffff","#00d7ff","#00afff","#0087ff","#5f5fff","#875fff"} // matches rowColors
  // web/site/assets/banner.json = {"frames":[...], "final":[6]string, "colors":[...], "tagline":"Open Channels, Handle Queries"}
  ```
  i18n JSON: flat `map[string]string`; template data `{Lang, Path, T map[string]string, Alternates []struct{Lang, URL string}, Version string}`; template parsed with `Option("missingkey=error")`.

- [ ] **Step 1: Failing test** `TestAnimationFrames` in `internal/ui`: `len == len(typing)+2*blinkTimes`; frame 0 rows equal `bannerLines("o","",true)` with colours off and trailing spaces trimmed; the blink "off" frames have no `caret`.
- [ ] **Step 2: Implement** `AnimationFrames` reusing `render`/caret logic (a `Printer{color:false}` is fine); run `go test ./internal/ui/` — PASS.
- [ ] **Step 3: Failing test** `TestBannerJSONInSync` in `internal/web/pages_test.go`: marshal the struct above (indent 2) and compare with `web/site/assets/banner.json`; with `-update` flag (`flag.Bool("update",…)`) it rewrites the file. Run with `-update` once to create it, then without — PASS.
- [ ] **Step 4: Failing page tests:**
  - `TestPageUz`: GET `/` → 200, `Content-Type: text/html; charset=utf-8`, body contains `<html lang="uz">`, `rel="canonical" href="https://ochq.uz/"`, `hreflang="x-default"`, section ids `hero`, `how`, `install`, `usage`, `about`, `comments`, the strings `brew install --cask mukhiddinov/tap/ochq`, `https://www.linkedin.com/in/khusniddin-mukhiddinov`, `Middle Software Engineer`; and does NOT contain `snap install`.
  - `TestInstallCommandsMatchReadme`: every install command shown on the page (`brew install --cask mukhiddinov/tap/ochq`, the `install.sh` curl line, `sudo dnf copr enable mukhiddinov/ochq`, `scoop bucket add ochq https://github.com/mukhiddinov/scoop-bucket`, `scoop install ochq`) is also present in `README.md` (read via `os.ReadFile("../../README.md")`).
  - `TestAssets`: GET `/assets/site.css` → 200 `text/css`, `Cache-Control` contains `max-age`; GET `/install.sh` → 302 to `https://mukhiddinov.github.io/ochq/install.sh`; GET `/nope` → 404.
  - `TestPageWeight`: sum of sizes of `page.html.tmpl` rendered for uz + `site.css` + `site.js` + `banner.json` < 100_000 bytes.
- [ ] **Step 5: Implement `pages.go`:** at `NewServer` time render each language once into `[]byte` (fail fast on template errors); serve `/` (uz) and, once Task 7 adds them, `/en/`, `/ru/` from a `map[string][]byte` keyed by path; `/assets/` via `http.FileServerFS(sub(FS,"site"))` with `Cache-Control: public, max-age=3600`; `/install.sh` redirect; `/robots.txt`, `/sitemap.xml` come in Task 7.
- [ ] **Step 6: Write `uz.json` and `page.html.tmpl`.** Sections in spec order (top bar, hero, how, chips, install, usage, about, comments, footer). Copy text from the mockups (already Uzbek) and the spec's developer section: name "Khusniddin Mukhiddinov", title "Middle Software Engineer", skills Backend — Java, Python, Go; DevOps — Linux, Docker, CI/CD (GitHub Actions), Traefik/Nginx, systemd; links GitHub + LinkedIn. Content is real HTML (SEO): step cards, commands in `<pre><code>`, all visible without JS. Head: title, description, OG/Twitter (`og.png`), canonical, three `hreflang` + `x-default`, Google Fonts `JetBrains Mono` 400/600/800 + `Inter` 400/500/600 with `display=swap`, `<link rel="icon" href="/favicon.svg">`. Strings for JS (ghost, "just now", errors, copy/copied) in `<script type="application/json" id="i18n">`.
- [ ] **Step 7: Run** `go test ./internal/web/ ./internal/ui/` — PASS.
- [ ] **Step 8: Commit** `feat(web): render the landing page skeleton` (no push yet — Tasks 4–6 land in the same PR).

### Task 4: Hero and "how it works" animations

**Files:** Modify `web/site/assets/site.js`, `web/site/assets/site.css`, `web/site/page.html.tmpl`.

- [ ] **Step 1:** Port hero mesh from `hero.html` `#a` and the story loop + faint mesh from `how-it-works.html` into `site.js` as `initHero(el)` and `initHow(el)`. Add: `IntersectionObserver` pauses each loop off-screen; `document.hidden` pauses; node count `min(W*H/9000, 140)` and half on `innerWidth < 640`; `matchMedia('(prefers-reduced-motion: reduce)')` → hero draws one static frame, how-it-works shows all steps `.on`, tunnel drawn, URL visible, no loop. Step captions come from the `#i18n` JSON.
- [ ] **Step 2: Verify in browser** (`go run ./cmd/ochq-web` with a temp salt/db, open `http://127.0.0.1:8090/`): animations match the mockups; DevTools Performance shows no long tasks > 50 ms while idle; with "Emulate CSS prefers-reduced-motion" both sections are static.
- [ ] **Step 3: Commit** `feat(web): animate the hero and the tunnel diagram`.

### Task 5: Install terminal and comments UI

**Files:** Modify `web/site/assets/site.js`, `site.css`, `page.html.tmpl`.

- [ ] **Step 1:** Port `install-terminal.html` as `initTerminal(el, banner)`; it must load frames/colours/tagline from `/assets/banner.json` (no hard-coded glyphs or the FIGlet port — the JSON is the single source). OS tab auto-selected from `navigator.userAgent` (Mac → macOS, Win → Windows, else Linux). Copy button uses `navigator.clipboard` with a `<textarea>` fallback. Reduced motion → final state rendered at once. Version string comes from template data (`{{.Version}}` = latest tag at build, passed via ldflags `-X …/version.Version`; fall back to hiding the version if `dev`).
- [ ] **Step 2:** `initComments(el)`: GET `/api/comments`, render each item with `document.createElement` + `textContent` (name or i18n "ghost", lang badge, relative time via `Intl.RelativeTimeFormat(lang)`); "show more" uses `?before=<last id>`; form posts JSON incl. hidden `website` honeypot and current `lang`; on 201 prepend; on 429/400 show the localised error; disable the button while sending.
- [ ] **Step 3: Check no HTML injection paths:** `grep -n "innerHTML" web/site/assets/site.js` — the only matches are in `initTerminal` writing trusted, escaped strings; none in `initComments`.
- [ ] **Step 4: Verify in browser:** post with and without name (ghost), post `<b>x</b>` shows literally, 4th post in 10 min shows the rate-limit message, tab switching restarts the terminal.
- [ ] **Step 5: Commit** `feat(web): add the install terminal and comments`.

### Task 6: Polish, accessibility, PR

- [ ] **Step 1:** Mobile: 16 px gutters, no horizontal scroll at 360 px (terminal `pre` scrolls inside itself), how-it-works stage above steps. Keyboard: tabs are `role="tablist"` buttons with arrow-key support; focus rings visible; colour contrast of muted text ≥ 4.5:1 on `#030712`.
- [ ] **Step 2: Verify:** Chrome DevTools device mode 360×740 — no horizontal scroll; Lighthouse mobile performance ≥ 90 and accessibility ≥ 90 (record scores in the PR body); reduced-motion emulation static.
- [ ] **Step 3:** `gofmt -l . && go vet ./... && go test -race ./...` — clean.
- [ ] **Step 4: Commit** `feat(web): mobile and accessibility polish`; push; PR "Landing page (Uzbek)" with screenshots; wait for merge.

---

## PR 4 — English, Russian, SEO files (branch `feat/landing-i18n`)

### Task 7: Translations and crawlers

**Files:** Create `web/i18n/en.json`, `web/i18n/ru.json`; modify `internal/web/pages.go`; test `internal/web/pages_test.go`.

- [ ] **Step 1: Failing tests:**
  - `TestI18nKeysMatch`: key sets of `uz.json`, `en.json`, `ru.json` are identical (report missing keys by name).
  - `TestPagesAllLangs` table over `/`→uz, `/en/`→en, `/ru/`→ru: `<html lang="…">`, canonical `https://ochq.uz/`, `https://ochq.uz/en/`, `https://ochq.uz/ru/`; each page links all three hreflangs + `x-default` → `https://ochq.uz/`.
  - `TestRedirectsNoSlash`: `/en` → 301 `/en/`.
  - `TestRobotsSitemap`: `/robots.txt` contains `Sitemap: https://ochq.uz/sitemap.xml`; `/sitemap.xml` lists the three URLs with `xhtml:link` alternates.
- [ ] **Step 2: Run** — FAIL. **Implement:** translate every value (natural, not literal; product terms `ochq`, `ochqd`, commands unchanged); generate robots/sitemap in Go at startup. Language switch links are plain `<a>`; `site.js` stores the choice in `localStorage` (try/catch) and on `/` only highlights the remembered language — no redirect.
- [ ] **Step 3: Run** `go test -race ./...` — PASS. Spot-check `/en/` and `/ru/` in the browser (no overflow from longer Russian strings at 360 px).
- [ ] **Step 4: Commit** `feat(web): add English and Russian pages, sitemap and robots`; PR; wait for merge.

---

## PR 5 — Deploy (branch `feat/deploy-ochq-web`) + vps-edge PR

### Task 8: Service, deploy script, CI, server setup

**Files:**
- Create: `deploy/ochq-web.service`, `deploy/ochq-web.env.example`, `deploy/ochq-web-deploy`, `scripts/setup-ochq-web.sh`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1:** `ochq-web.service`: copy `deploy/ochqd.service` hardening block verbatim; `User=ochq_web`, `StateDirectory=ochq-web`, `EnvironmentFile=/etc/ochq/ochq-web.env`, `ExecStart=/opt/ochq-web/current/ochq-web serve`, `MemoryMax=128M`, `TasksMax=64`, `After=docker.service network-online.target` and `Restart=always`/`RestartSec=5s` (the listen address belongs to the Docker `edge` bridge, which exists only once Docker is up). `env.example`: `OCHQ_WEB_ADDR=172.30.0.1:8090` (edge network gateway, not public), `OCHQ_WEB_TRUSTED_PROXY=172.30.0.2`, `OCHQ_WEB_SALT=` with a comment `generate: openssl rand -hex 32`.
- [ ] **Step 2:** `ochq-web-deploy`: same structure as `deploy/ochq-deploy` with `BASE=/opt/ochq-web`, binary `ochq-web`, smoke test `runuser -u ochq_web -- … -version` expecting `ochq-web *`, `systemctl restart ochq-web`, health check = `systemctl is-active` **and** `curl -fsS "http://${OCHQ_WEB_ADDR}/healthz"` (read addr from the env file), rollback identical. Do not edit `deploy/ochq-deploy`.
- [ ] **Step 3:** `setup-ochq-web.sh` (idempotent, root): `bash -n` clean; validates the pubkey with `ssh-keygen -lf`; creates `ochq_web` (system, nologin) and `ochq_web_deploy`; installs the deploy script to `/usr/local/bin/`, sudoers `/etc/sudoers.d/ochq-web-deploy` allowing only that script (validated with `visudo -cf`); forced-command `authorized_keys` like `setup-vps.sh`; installs the unit; creates `/etc/ochq/ochq-web.env` from the example with a generated salt if absent (never overwrites); reads the `edge` network gateway with `docker network inspect edge -f '{{(index .IPAM.Config 0).Gateway}}'` and refuses to continue unless it is `172.30.0.1` (else prints what to set in the env file); checks port 8090 is free with `ss -ltn`; prints (does not run) `ufw allow from 172.30.0.2 to any port 8090 proto tcp`.
- [ ] **Step 4:** `ci.yml`: add a `changes` job (`dorny/paths-filter@v3`) with filters `ochqd: [cmd/ochqd/**, internal/server/**, internal/proto/**, internal/tlsutil/**, pkg/**, go.mod, go.sum]` and `web: [cmd/ochq-web/**, internal/web/**, internal/ui/**, web/**, pkg/**, go.mod, go.sum]`. Existing `deploy` gets `needs: [test, changes]` and `if: … && (needs.changes.outputs.ochqd == 'true' || github.event_name == 'workflow_dispatch')`. New `deploy-web` job mirrors it with secrets `WEB_DEPLOY_SSH_KEY` (reuses `DEPLOY_KNOWN_HOSTS`), user `ochq_web_deploy`, builds `./cmd/ochq-web` with version = latest tag (`git describe --tags --abbrev=0`, checkout with `fetch-depth: 0`), concurrency group `production-web`.
- [ ] **Step 5: Verify:** `bash -n deploy/ochq-web-deploy scripts/setup-ochq-web.sh`; `shellcheck` both if installed; `actionlint .github/workflows/ci.yml` if installed (else `python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))' .github/workflows/ci.yml`); `systemd-analyze verify deploy/ochq-web.service` (ignore "executable not found").
- [ ] **Step 6: Commit** `feat(deploy): ship ochq-web separately from ochqd`; PR body includes the one-time server steps in order (below); wait for merge **after** the user has done the server steps and added the secret.

One-time server steps for the user (Uzbek explanation in chat, commands as a script, no long pasted one-liners):
1. `ssh-keygen -t ed25519 -f ~/ochq-web-deploy -N '' -C ochq-web-deploy` locally; `gh secret set WEB_DEPLOY_SSH_KEY < ~/ochq-web-deploy`.
2. On VPS: `git -C /root/ochq pull` (or clone), `sudo bash scripts/setup-ochq-web.sh "$(cat ochq-web-deploy.pub)"`, then the printed `ufw` command.
3. Merge PR 5 → CI deploys → `curl -s http://172.30.0.1:8090/healthz` on VPS prints `ok`.

### Task 9: Traefik route (repo `~/Desktop/projects/vps-edge`, separate PR)

**Files:** Create `traefik/dynamic/ochq-web.yml`.

- [ ] **Step 1:** Router `ochq-web`: `rule: Host(\`ochq.uz\`) || Host(\`www.ochq.uz\`)`, `priority: 100` (above the wildcard `ochq` router, whose regex also matches `www`), `entryPoints: [websecure]`, middleware `ochq-www-redirect` (`redirectRegex` `^https://www\.ochq\.uz/(.*)` → `https://ochq.uz/${1}`, permanent), same `{{ if env "CF_DNS_API_TOKEN" }}` TLS block as `ochq.yml`; service `ochq-web` → `http://172.30.0.1:8090` (the `edge` network's gateway, where ochq-web listens).
- [ ] **Step 2: Verify:** the repo's CI/validate script passes (`scripts/` in vps-edge); after deploy `curl -sI https://ochq.uz` → 200, `curl -sI https://www.ochq.uz` → 301 to `https://ochq.uz/`, `curl -sI https://a7f3.ochq.uz` still reaches ochqd (404 "tunnel not found" text from ochqd, not Traefik).
- [ ] **Step 3: Commit** `feat: route ochq.uz to ochq-web`; PR in vps-edge; user merges after PR 5 is deployed.

---

## PR 6 — Links (branch `docs/link-landing`)

### Task 10: README and short install URL

- [ ] **Step 1:** README: add `https://ochq.uz` near the top (badge/link line) and show `curl -fsSL https://ochq.uz/install.sh | sh` as the primary install line, keeping the GitHub Pages URL as "alternative". Update `TestInstallCommandsMatchReadme` command list if the page changes to the short URL too (it should, in all three i18n files).
- [ ] **Step 2: Verify:** `go test ./internal/web/` PASS; `curl -fsSL https://ochq.uz/install.sh | head -3` prints the script header.
- [ ] **Step 3: Commit** `docs: link ochq.uz and the short install URL`; PR; wait for merge.
