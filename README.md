<div align="center">

# OCHQ

**Open Channels, Handle Queries**

Expose a local port to the internet under a public URL — one command, no router setup.

[![CI](https://github.com/mukhiddinov/ochq/actions/workflows/ci.yml/badge.svg)](https://github.com/mukhiddinov/ochq/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mukhiddinov/ochq?sort=semver)](https://github.com/mukhiddinov/ochq/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/mukhiddinov/ochq)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

</div>

```text
$ ochq 3000 --sub demo
            _
  ___   ___| |__   __ _
 / _ \ / __| '_ \ / _` |
| (_) | (__| | | | (_| |
 \___/ \___|_| |_|\__, |   0.3.4
                     |_|   Open Channels, Handle Queries

[✓] Status:       Online
[✓] Forwarding:   http://localhost:3000
[✓] Public URL:   https://demo.ochq.uz

Logs (Ctrl+C to stop):
14:02:07  GET    /api/v1/health        200 OK              14ms
14:02:09  POST   /auth/login           401 Unauthorized    42ms
```

*ochq* comes from the Uzbek word *ochiq* — "open". It is a lightweight,
self-hostable reverse tunnel in the spirit of ngrok: the `ochq` client connects
out to an `ochqd` server, and every request to `https://<name>.ochq.uz` is
carried back through that connection to your `localhost`.

> [!NOTE]
> OCHQ is in **preview** (v0.x). The hosted server at `ochq.uz` is free and
> needs no account — install `ochq` and run it. Accounts with personal tokens
> come later. You can also [run your own server](docs/self-hosting.md).

## Contents

- [Features](#features)
- [Installation](#installation) — [Linux](#linux) · [macOS](#macos) · [Windows](#windows) · [From source](#from-source)
- [Quick start](#quick-start)
- [Usage](#usage)
- [Updating and uninstalling](#updating-and-uninstalling)
- [How it works](#how-it-works)
- [Security](#security)
- [Self-hosting](#self-hosting)
- [Development](#development)
- [Roadmap](#roadmap)
- [License](#license)

## Features

- **One command, zero setup** — `ochq 3000` and your local app is online. No
  account, token or config file.
- **Custom subdomains** — `--sub my-api` gives you `https://my-api.ochq.uz`.
- **Live request log** — method, path, status and latency of every request.
- **Survives network drops** — reconnects with exponential backoff and keeps
  the same URL.
- **WebSockets and streaming** — upgrades, server-sent events and long polling
  pass through untouched.
- **Encrypted tunnel** — TLS 1.3 between client and server, with certificate
  pinning.
- **Single static binary** — Linux, macOS and Windows, on amd64 and arm64.
- **Native packages** — install and update with `apt`, `dnf` (Fedora COPR or
  the signed OCHQ repository), Homebrew or Scoop.

## Installation

### Linux

| Distribution | Install |
|---|---|
| **Fedora** 43+ | `sudo dnf copr enable mukhiddinov/ochq && sudo dnf install ochq` |
| **Debian / Ubuntu** and derivatives | `curl -fsSL https://mukhiddinov.github.io/ochq/install.sh \| sh` |
| **RHEL / Rocky / Alma** | `curl -fsSL https://mukhiddinov.github.io/ochq/install.sh \| sh` |
| Anything else | the same installer, or a [release archive](https://github.com/mukhiddinov/ochq/releases/latest) (see below) |

**Fedora** — the package is built from source on Fedora's own
[COPR](https://copr.fedorainfracloud.org/coprs/mukhiddinov/ochq/) builders
for x86_64 and aarch64. `dnf copr enable` adds the repository once (dnf asks
you to confirm, as for every COPR); after that `dnf upgrade` keeps `ochq`
current.

```bash
sudo dnf copr enable mukhiddinov/ochq
sudo dnf install ochq
```

**One-line installer** — on Debian, Ubuntu, RHEL and their derivatives it adds
the signed OCHQ package repository and installs `ochq` with `apt` or `dnf`, so
regular system updates keep it up to date. On other distributions it installs
the checksum-verified binary to `/usr/local/bin`.

```bash
curl -fsSL https://mukhiddinov.github.io/ochq/install.sh | sh
```

<details>
<summary><b>Debian / Ubuntu (apt), step by step</b></summary>

```bash
# 1. Trust the OCHQ signing key
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://mukhiddinov.github.io/ochq/gpg.key | sudo tee /etc/apt/keyrings/ochq.asc >/dev/null

# 2. Add the repository
echo "deb [signed-by=/etc/apt/keyrings/ochq.asc] https://mukhiddinov.github.io/ochq/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/ochq.list

# 3. Install
sudo apt update
sudo apt install ochq
```

Requires apt 2.4 or newer (Debian 12+, Ubuntu 22.04+).
</details>

<details>
<summary><b>RHEL / Rocky / Alma (dnf), step by step</b></summary>

The OCHQ repository works on Fedora too, if you prefer it to COPR — use one
or the other, not both.

```bash
# 1. Add the repository (the signing key is fetched on first use)
curl -fsSL https://mukhiddinov.github.io/ochq/rpm/ochq.repo | sudo tee /etc/yum.repos.d/ochq.repo

# 2. Install — dnf asks you to confirm the key; check its fingerprint (below)
sudo dnf install ochq
```
</details>

<details>
<summary><b>Any distribution — manual download</b></summary>

Download the archive for your architecture from the
[latest release](https://github.com/mukhiddinov/ochq/releases/latest), verify
it against `checksums.txt`, and put the binary on your `PATH`:

```bash
VERSION=0.3.4 ARCH=amd64   # or arm64
curl -fsSLO https://github.com/mukhiddinov/ochq/releases/download/v$VERSION/ochq_${VERSION}_linux_$ARCH.tar.gz
curl -fsSLO https://github.com/mukhiddinov/ochq/releases/download/v$VERSION/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf ochq_${VERSION}_linux_$ARCH.tar.gz ochq
sudo install -m 0755 ochq /usr/local/bin/ochq
```
</details>

The OCHQ repository's packages and metadata are signed with this key (COPR
packages are signed by COPR's own key) — compare it with what `apt`/`dnf`
show you:

```text
OCHQ Package Signing
C3BB BF58 2B11 C9BA 6C06  638E 0FF4 EA88 4839 27A1
```

### macOS

With [Homebrew](https://brew.sh):

```bash
brew install --cask mukhiddinov/tap/ochq
```

The binary is not notarized by Apple yet; the cask clears the quarantine flag
so Gatekeeper lets it run. Both Apple Silicon and Intel Macs are supported.

<details>
<summary><b>Without Homebrew</b></summary>

```bash
curl -fsSL https://mukhiddinov.github.io/ochq/install.sh | sh
```

or download `ochq_<version>_darwin_arm64.tar.gz` (Apple Silicon) /
`ochq_<version>_darwin_amd64.tar.gz` (Intel) from the
[releases page](https://github.com/mukhiddinov/ochq/releases/latest), then:

```bash
tar -xzf ochq_*_darwin_*.tar.gz ochq
xattr -d com.apple.quarantine ochq
sudo install -m 0755 ochq /usr/local/bin/ochq
```
</details>

### Windows

With [Scoop](https://scoop.sh) (PowerShell):

```powershell
scoop bucket add ochq https://github.com/mukhiddinov/scoop-bucket
scoop install ochq
```

<details>
<summary><b>Without Scoop</b></summary>

1. Download `ochq_<version>_windows_amd64.zip` (or `_arm64.zip`) from the
   [releases page](https://github.com/mukhiddinov/ochq/releases/latest).
2. Check it against `checksums.txt`:
   ```powershell
   Get-FileHash .\ochq_0.3.4_windows_amd64.zip -Algorithm SHA256
   ```
3. Extract `ochq.exe` into a folder on your `PATH`, e.g. `%USERPROFILE%\bin`.

The executable is not code-signed yet, so SmartScreen may warn on first run —
choose **More info → Run anyway**.
</details>

### From source

Requires Go 1.26+:

```bash
go install github.com/mukhiddinov/ochq/cmd/ochq@latest
```

Check the installation on any platform:

```bash
ochq version
```

## Quick start

**1. Start something locally**, for example:

```bash
python3 -m http.server 3000
```

**2. Open the tunnel:**

```bash
ochq 3000
```

That's all — `ochq` prints your public URL, such as `https://k3x9.ochq.uz`.
Open it from any device; your phone on mobile data is a good test.

**3. Want a fixed name?** Ask for a subdomain:

```bash
ochq 3000 --sub my-app        # https://my-app.ochq.uz
```

## Usage

```text
ochq [http] <port | host:port> [flags]
```

| Example | What it does |
|---|---|
| `ochq 3000` | Expose `localhost:3000` on a random subdomain |
| `ochq http 8080` | Same, with the protocol spelled out |
| `ochq 3000 --sub my-api` | Ask for `my-api.ochq.uz` (3–63 letters, digits or hyphens; `--sub my-api.ochq.uz` works too) |
| `ochq 192.168.1.20:80` | Expose another machine on your network |

Flags may come before or after the port.

| Flag | Description |
|---|---|
| `--sub` | Requested subdomain; random if omitted |
| — | Set `NO_COLOR` to disable colored output ([no-color.org](https://no-color.org)) |

If a subdomain is already in use, `ochq` says so and exits — pick another name
or leave `--sub` out for a random one.

<details>
<summary><b>Using your own ochqd server</b></summary>

`ochq` talks to the hosted server at `tunnel.ochq.uz` out of the box. For a
[self-hosted](docs/self-hosting.md) server, pass its address and, if it needs
them, its token and fingerprint. Each flag has an environment variable; the
flag wins if both are set.

| Flag | Environment | Description |
|---|---|---|
| `--server` | `OCHQ_SERVER` | `ochqd` tunnel address, `host:port` (default `tunnel.ochq.uz:9000`) |
| `--token` | `OCHQ_TOKEN` | Access token, if the server requires one |
| `--fingerprint` | `OCHQ_FINGERPRINT` | Pin the server's self-signed TLS key (`SHA256:...`) |

```bash
ochq 3000 --server tunnel.example.com:9000 --token "$TOKEN" --fingerprint SHA256:...
```

The hosted server's own token and fingerprint are built into `ochq` and are
never sent to any other server.
</details>

Other commands:

| Command | Description |
|---|---|
| `ochq help [command]` | Help for `ochq` or one of its commands (also `-h`, `--help`) |
| `ochq version` | Print the version (also `--version`) |
| `ochq completion <shell>` | Print a completion script for `bash`, `zsh`, `fish` or `powershell` |

### Shell completion

Press <kbd>Tab</kbd> to complete commands and flags. The apt and dnf packages
and the Homebrew cask install completion for bash, zsh and fish automatically —
open a new shell. Otherwise:

```bash
# bash (needs the bash-completion package)
ochq completion bash | sudo tee /etc/bash_completion.d/ochq >/dev/null
# zsh
ochq completion zsh > "${fpath[1]}/_ochq"
# fish
ochq completion fish > ~/.config/fish/completions/ochq.fish
```

```powershell
# PowerShell: load it in every session
Add-Content $PROFILE 'ochq completion powershell | Out-String | Invoke-Expression'
```

**What you see.** Every completed request becomes one log line with its time,
method, path, status and duration. If the connection to the server drops,
`ochq` shows `Reconnecting` with the next retry delay and comes back on the
same URL. If nothing is listening on your local port, `ochq` warns at startup
and visitors get a `502` explaining that the local service is unreachable.

**Exit codes.** `0` after Ctrl+C, `1` on an error that retrying cannot fix
(invalid or taken subdomain, fingerprint mismatch, wrong token). Such errors
come with a `hint:` line.

## Updating and uninstalling

| Platform | Update | Uninstall |
|---|---|---|
| Debian / Ubuntu | `sudo apt update && sudo apt upgrade` | `sudo apt remove ochq` |
| Fedora (COPR) | `sudo dnf upgrade --refresh ochq` | `sudo dnf remove ochq` |
| RHEL / Rocky / Alma | `sudo dnf upgrade --refresh ochq` | `sudo dnf remove ochq` |
| macOS (Homebrew) | `brew upgrade --cask ochq` | `brew uninstall --cask ochq` |
| Windows (Scoop) | `scoop update ochq` | `scoop uninstall ochq` |
| Manual install | re-run the installer | `sudo rm /usr/local/bin/ochq` |

To remove the package repository as well:
`sudo dnf copr remove mukhiddinov/ochq` (COPR),
`sudo rm /etc/apt/sources.list.d/ochq.list /etc/apt/keyrings/ochq.asc` (apt) or
`sudo rm /etc/yum.repos.d/ochq.repo` (OCHQ dnf repository).

## How it works

```text
 Browser ── https://demo.ochq.uz ──▶  edge proxy (TLS, *.ochq.uz certificate)
                                              │
                                              ▼
                                           ochqd ── picks the tunnel by Host header
                                              │
                    one TLS 1.3 connection, many multiplexed streams (yamux)
                                              │
 Your app ◀── http://localhost:3000 ──────  ochq
```

1. `ochq` dials `ochqd` on port 9000 over TLS 1.3 and sends a handshake with the
   protocol version, the requested subdomain and the token.
2. `ochqd` reserves the subdomain and answers with the public URL. From then on
   the connection carries a [yamux](https://github.com/hashicorp/yamux)
   session: many independent streams over one TCP connection, so slow requests
   never block fast ones.
3. For every public request `ochqd` opens a new stream; `ochq` proxies it to
   your local service and streams the response back.
4. Keepalive pings detect dead connections within seconds; the subdomain is
   then freed, or kept if the client reconnects.

Details of the wire protocol are in [`internal/proto`](internal/proto).

## Security

- **Tunnel encryption.** Client ↔ server traffic, including the token, is TLS 1.3.
- **Server identity.** The client pins the server's public key, like SSH host
  keys: the hosted server's key is built into `ochq`, so an impostor on the
  network is refused before anything is sent. For a self-hosted server with a
  self-signed certificate, pass its key with `--fingerprint`; without a pin
  `ochq` refuses the connection and prints the fingerprint it saw.
- **What you expose is public.** Anyone with the URL can reach your local
  service. Don't tunnel admin panels, databases or a directory with secrets —
  `python3 -m http.server` in a project folder publishes its `.git` too.
- **Supply chain.** Releases are built by GitHub Actions from tagged commits;
  archives come with `checksums.txt`, and the apt/dnf repositories are signed
  (see the key fingerprint above).

Please report vulnerabilities privately through
[GitHub Security Advisories](https://github.com/mukhiddinov/ochq/security/advisories/new),
not in public issues.

## Self-hosting

`ochqd` is a single binary you can run on any Linux server with a wildcard DNS
record. See **[docs/self-hosting.md](docs/self-hosting.md)** for DNS, TLS,
systemd, the reverse proxy in front and the restricted CI deploy user.

## Development

```bash
git clone https://github.com/mukhiddinov/ochq.git && cd ochq
go test -race ./...
```

Run everything locally — `*.localhost` always resolves to `127.0.0.1`:

```bash
go run ./cmd/ochqd                                      # terminal 1, prints its fingerprint
python3 -m http.server 3000                             # terminal 2
go run ./cmd/ochq 3000 --sub demo --server localhost:9000 --fingerprint SHA256:...   # terminal 3
curl http://demo.localhost:8080/                        # terminal 4
```

| Path | Contents |
|---|---|
| `cmd/ochq`, `cmd/ochqd` | Client and server entry points |
| `internal/proto` | Handshake messages and framing |
| `internal/client` | Tunnel runner, local proxy, reconnect |
| `internal/server` | Tunnel registry and public HTTP routing |
| `internal/tlsutil` | Certificates and fingerprint pinning |
| `internal/ui` | Terminal output |
| `deploy/`, `scripts/` | systemd unit, deploy and server setup |
| `packaging/` | apt/dnf repository builder and `install.sh` |

**Contributing.** Open an issue first for larger changes. Commits follow
[Conventional Commits](https://www.conventionalcommits.org) (`feat:`, `fix:`,
`docs:` …) — the release changelog is generated from them.

**Releasing.** `git tag -a vX.Y.Z -m "..." && git push origin vX.Y.Z`. CI tests
the tag, builds every platform with GoReleaser, publishes the GitHub release,
updates Homebrew and Scoop, and rebuilds the signed apt/dnf repository.

## Roadmap

- [x] TLS tunnel with multiplexed streams
- [x] Live request log, automatic reconnect
- [x] Signed packages for apt, dnf, Homebrew and Scoop; Fedora COPR
- [ ] Snap Store (name registration under review)
- [x] Free hosted server, no setup
- [ ] Accounts: `ochq login` opens the browser, personal tokens, request history
- [ ] Reserved subdomains per account
- [ ] Local web inspector at `localhost:4040`: view and replay requests
- [ ] TCP tunnels

## License

[MIT](LICENSE) © Mukhiddinov
