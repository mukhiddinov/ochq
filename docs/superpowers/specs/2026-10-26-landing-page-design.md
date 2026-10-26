# ochq.uz landing page — design

Status: draft for approval · 2026-10-26

## Intent

ochq is now free and zero-config (`ochq 3000` just works). People arrive from
a Telegram post or a search engine and today land on a GitHub README. We want
a real home at `https://ochq.uz` that:

- makes a visitor *feel* the product in the first seconds: a dark room, deep
  blue, signals running through a neural network — "as if you fell inside the
  network";
- answers "what can it do?" and "how does it work?" without reading docs;
- gets them installed on Linux, macOS or Windows in one copy-paste;
- says who built it;
- lets them leave a comment, with a name or anonymously ("ghost");
- is findable by search engines in Uzbek, English and Russian.

## Goals

1. One landing page in three languages: Uzbek, English, Russian.
2. Hero with the animated neural network (approved mockup "A").
3. "How it works" section: a 4-step poster synchronised with an animated
   network diagram (approved mockup).
4. "What it can do" use-case chips.
5. Install instructions per OS + usage examples, matching README.md.
6. Developer section.
7. Comments: name optional, anonymous shows as "ghost".
8. SEO: real HTML content per language, meta/OG tags, hreflang, sitemap,
   robots.txt.
9. Light: no frontend framework, no build step required to view the page.

## Non-goals (for now)

- Accounts/login, comment replies, reactions, editing comments.
- An admin web UI (moderation is a CLI command on the VPS).
- Analytics/tracking scripts, cookies banners.
- Docs site / blog.

## Anti-goals (would be a failure even if it "works")

- Heavy page: a framework bundle, video files, or animation that makes a
  laptop fan spin or a phone stutter.
- Generic template look (purple gradient SaaS page). The look is the
  approved dark neural network, deep blue + cyan.
- A comment box that fills with spam or lets someone inject HTML/scripts.
- Anything that touches other projects on the shared VPS (efbazaar,
  examiner, mailcow) or a tunnel being able to hijack the landing.
- Install commands on the page that differ from README.md.

## Approved visual direction

Mockups (kept locally in `.superpowers/brainstorm/…/content/`, not committed):
`hero.html` (option A chosen) and `how-it-works.html`. The builder should
copy their look and animation logic, not reinvent it.

- Palette: background `#030712`, panels `#070f20`, lines `#1a2a4a`, ink
  `#e6f0ff`, muted `#7d8fb3`, cyan `#38e1ff`, blue `#2f6bff`; request packets
  violet `#a98bff`, response packets green `#7dffb2`.
- Type: JetBrains Mono for logo/headings/code, Inter for body (Google Fonts,
  `display=swap`).
- Hero: full-screen canvas; nodes drift, links between near nodes, cyan
  pulses travel along links; near the mouse the network brightens and pulses
  more. Copy: big `ochq_` logo, one-line tagline with `https://nomi.ochq.uz`,
  `$ ochq 3000` command with a copy button.
- How it works: left = 4 step cards, the active one glows; right = SVG stage:
  laptop (app `:3000` + ochq client) → NAT/firewall dashed line → tunnel tube
  → `ochqd · tunnel.ochq.uz` → three visitors (browser, phone, webhook).
  Loop: (1) `ochq 3000` is typed, the tunnel draws outward through the
  firewall; (2) handshake pulse there and back, `https://demo.ochq.uz`
  appears; (3) three requests travel visitor → server → tunnel → app in
  parallel; (4) responses travel back. Two rounds, pause, restart. Faint mesh
  in the background ties it to the hero.

## Page structure (top to bottom)

1. Top bar: logo, anchor links (How it works · Install · Comments), language
   switch `UZ · EN · RU`, GitHub link with star call-to-action.
2. Hero.
3. How it works (poster + animation).
4. What it can do — chips: test Telegram bot webhooks locally; show a site to
   a client without deploying; test on a phone with real HTTPS; your own
   subdomain (`--sub`); self-host with `ochqd`.
5. Install — tabs macOS / Linux / Windows (auto-select from the visitor's OS),
   each command with a copy button. Centre piece is an animated terminal
   (approved mockup `install-terminal.html`, inspired by a reference video
   the user shared): a macOS-style window types the install command, plays
   realistic package-manager output with a progress bar, then types
   `ochq 3000` and plays the REAL banner animation — the FIGlet glyphs,
   kerning/smushing, typing frames (o, oc, och, ochi, ochiq, ochi, och,
   ochq), the drawn caret blinking twice, row colours and version/tagline,
   all ported 1:1 from `internal/ui/banner.go` — then `Connecting…`, the
   `[✓] Status / Forwarding / Public URL` block and a few request log lines,
   then loops. Switching the tab restarts it with that OS's command and
   output. The banner port must stay in sync with banner.go (a test or a
   build step that compares them is welcome). Linux shows the per-distribution choices
   from README.md (install.sh, Fedora COPR). Snap is NOT shown until the
   Snap Store approves the name.
6. Usage — `ochq 3000`, `ochq 3000 --sub mening-saytim`, link to README for
   self-hosting.
7. Developer — Khusniddin Mukhiddinov, Middle Software Engineer. Short bio
   (one–two sentences: author of ochq). Skills: Backend — Java, Python, Go;
   DevOps — Linux, Docker, CI/CD (GitHub Actions), Traefik/Nginx, systemd.
   Links: GitHub `https://github.com/mukhiddinov`, LinkedIn
   `https://www.linkedin.com/in/khusniddin-mukhiddinov`. No photo.
8. Comments — one shared thread for all three languages (user's decision);
   each comment shows a small `UZ`/`EN`/`RU` tag of the page it was written
   on. List (newest first, paginated "show more") + form: name
   (optional, max 40) and comment (required, max 1000). Empty name → "ghost".
9. Footer: MIT licence, GitHub, "made in Uzbekistan". No release version
   (it would need a third-party request or go stale). (Claude's call.)

## Languages and URLs

- `https://ochq.uz/` — Uzbek (Latin script), the x-default.
- `https://ochq.uz/en/` — English.
- `https://ochq.uz/ru/` — Russian.
- Each is a full static HTML page with its own `<title>`, description,
  `lang` attribute, canonical URL, OG/Twitter tags and `hreflang` links to
  the other two. The language switch is plain links; the chosen language is
  remembered in `localStorage` (best-effort, wrapped in try/catch) only to
  pre-select on the next visit to `/` — no automatic redirect that would
  confuse search engines. (Claude's call.)
- Translations are written by Claude; Uzbek is the source text.
- Comment UI strings are translated; comment content is shown as written.
- `robots.txt` and `sitemap.xml` listing the three URLs.
- `www.ochq.uz` redirects permanently to `ochq.uz`.

## Architecture

### Where it lives

- In this repo: `web/` holds the static site (HTML per language, CSS, JS,
  icons, OG image). A small Go program `cmd/ochq-web` serves `web/` (embedded
  with `embed.FS`, so one binary is the whole deploy) and the comments API.
- Why not GitHub Pages: comments need a server, and the apex `ochq.uz` should
  be ours for SEO. Why not inside `ochqd`: deploying the landing must not
  restart people's tunnels, and the tunnel server should stay small. (Claude's
  call.)

### Routing on the VPS

- Traefik (repo `vps-edge`, private) gets a router `Host(\`ochq.uz\`) ||
  Host(\`www.ochq.uz\`)` → `ochq-web`, with higher priority than the existing
  wildcard `ochq` router; `www` → apex redirect middleware. The existing
  wildcard certificate already covers `ochq.uz`.
- Today `ochq.uz` returns Traefik's 404, so nothing breaks.
- `ochqd` must refuse to register reserved subdomains so a tunnel can never
  appear as `www`, `api`, `admin`, `mail`, `login`, `status`, `docs`, `blog`,
  `app`, `dashboard`, `ochq`, `tunnel` (control endpoint), `pkg` (planned
  package host), `demo` (shown in the animation — must not be claimable for
  phishing). List in code, easy to extend. Reserved names are refused at
  registration only; nothing needs evicting because the registry is
  in-memory and the deploy restart clears it. (Claude's call.) This is a
  small separate change in `internal/server` with tests.

### ochq-web service

- Runs on the host as systemd unit `ochq-web` under its own user
  (`ochq_web`, or reuse `ochq` — builder's call, but namespaced under ochq),
  listening on a loopback-reachable port for Traefik the same way ochqd does
  (`host.docker.internal`), port chosen not to clash (check `ss -ltnp`).
- Data: SQLite file under `/var/lib/ochq-web/` (pure-Go driver,
  `modernc.org/sqlite`, so cross-compiling stays CGO-free). Schema: `comments(id,
  name, body, lang, created_at, ip_hash, hidden)`.
- Deploy: same pattern as ochqd — CI on push to main builds the binary and
  ships it over SSH to a forced-command deploy script (`ochq-web-deploy`)
  with atomic symlink switch and rollback on failed health check. It gets
  its own deploy user `ochq_web_deploy`, its own SSH key and its own GitHub
  secrets, so the ochqd deploy path is not touched. (Claude's call.)
- CI deploys by changed paths: changes under `web/`, `cmd/ochq-web/`,
  `internal/web/` deploy only ochq-web; ochqd deploys only when its own code
  changes, so landing PRs never restart people's tunnels. (Claude's call.)
- Config by env file `/etc/ochq/ochq-web.env` (listen address, DB path,
  IP-hash salt).

### Comments API

- `GET /api/comments?before=<id>&limit=20` → JSON list, newest first, hidden
  ones excluded.
- `POST /api/comments` JSON `{name, body, website}` → 201 with the created
  comment.
- Validation: trim; body 1–1000 runes, name 0–40 runes; empty name stored as
  empty, shown as "ghost" (localised). Reject control characters.
- Rendering: the page inserts comments with `textContent` only — never
  `innerHTML`. Links are not clickable. Newlines preserved with CSS.
- Spam protection:
  - honeypot field `website` (hidden); filled → pretend success, store
    nothing;
  - rate limit per client IP: 3 comments / 10 minutes, plus a global cap
    (e.g. 60 / hour) as a flood fuse;
  - client IP from `X-Forwarded-For` trusted only from Traefik's address
    (172.30.0.2), same rule as the other projects; stored only as a salted
    hash;
  - request body size limit (8 KB).
- Moderation: `ochq-web hide <id>` / `ochq-web unhide <id>` / `ochq-web list`
  subcommands run on the VPS against the same DB.

### Front-end behaviour

- Vanilla JS, no framework, no bundler. One CSS file, one JS file shared by
  the three language pages; strings that JS needs come from a small
  per-page JSON block.
- Animations: `requestAnimationFrame`, paused when the section is off-screen
  (IntersectionObserver) or the tab is hidden; fewer nodes on small screens;
  `prefers-reduced-motion` → static frame (hero shows the network still,
  "how it works" shows all four steps without motion).
- Page works with JS disabled except comments and animations (content is in
  the HTML for SEO).
- Mobile: 16 px side gutters, no horizontal scroll; how-it-works stage above
  the step cards.
- Budget: the whole page (HTML+CSS+JS, excluding fonts) under ~100 KB
  uncompressed; Lighthouse performance and accessibility ≥ 90 on mobile.

### Bonus: short install URL

`https://ochq.uz/install.sh` serves the same script as
`mukhiddinov.github.io/ochq/install.sh` (copied into the binary at build
time, or redirected). The page and README may then show
`curl -fsSL https://ochq.uz/install.sh | sh`; the old URL keeps working.
(Claude's call; drop it if it complicates the release pipeline.)

## Delivery

Small PRs, one step each, tests in every PR, the user merges, CI deploys:

1. `ochqd` reserved subdomains.
2. `cmd/ochq-web`: static serving + comments API + SQLite + moderation CLI,
   with tests.
3. Static site in Uzbek (all sections, animations).
4. English and Russian pages, SEO files.
5. Deploy: systemd unit, deploy script, CI job; `vps-edge` router (separate
   PR in that repo). One-time server setup commands given to the user.
6. README link to the site (and short install URL if done).

## Constraints

- Shared VPS: never touch other projects' users, ports, containers or
  configs; everything named `ochq*`.
- Never put secrets in the repo or in chat; the IP-hash salt is generated on
  the server.
- Commits and PRs carry no AI attribution lines.
- Explanations to the user in Uzbek; code, comments and README in English.

## Open items

None.

## Left to the builder

File layout inside `web/`, exact wording of copy (keeping the meaning above),
OG image design, port number, Go package structure, test approach.
