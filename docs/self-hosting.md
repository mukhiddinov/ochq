# Self-hosting ochqd

`ochqd` is the OCHQ server: one static binary that accepts `ochq` clients and
routes public HTTP traffic to them by subdomain. This guide sets it up on a
Linux server under your own domain.

```text
Internet ──▶ reverse proxy :443 (TLS, *.example.com) ──▶ ochqd :8080 (HTTP, routes by Host)
                                                           ▲
ochq clients ─────────── TLS 1.3 ──────────────────▶ ochqd :9000 (tunnels)
```

## Requirements

- A Linux server with a public IP (systemd assumed below).
- A domain whose DNS you control, for example `example.com`.
- Ports `9000/tcp` (tunnels) reachable from the internet, and `443/tcp` on the
  reverse proxy.
- Go 1.26+ to build `ochqd`, or a CI pipeline that builds it (see below).

## 1. DNS

| Type | Name | Value |
|---|---|---|
| A | `*.example.com` | server IP |
| A | `tunnel.example.com` | server IP |

The wildcard sends every tunnel subdomain to the server; `tunnel.` is the
address clients connect to. If the DNS is on Cloudflare, keep these records
**DNS only** (grey cloud): Cloudflare's proxy does not carry port 9000.

## 2. Build and install

```bash
go build -trimpath -ldflags "-s -w" -o ochqd ./cmd/ochqd
sudo install -m 0755 ochqd /usr/local/bin/ochqd
ochqd -version
```

For a production setup with a dedicated service user, a sandboxed systemd unit
and continuous deployment, use the files this repository ships:

```bash
# on the server, as root — creates the users ochq and ochq_deploy,
# /opt/ochq, /etc/ochq/ochqd.env and the systemd unit
sudo bash scripts/setup-vps.sh "$(cat ochq_deploy.pub)"
```

`ochq_deploy` can only run the deploy command (an SSH forced command), so a
leaked CI key can neither open a shell nor forward ports. The `ci` workflow
then builds `ochqd` on every push to `main` and installs it through that key;
the deploy checks the upload is a working Linux binary and rolls back if the
new release does not start. Set the repository variable `DEPLOY_HOST` and the
secrets `DEPLOY_SSH_KEY` and `DEPLOY_KNOWN_HOSTS` to enable it.

## 3. Configure

All flags have defaults; the systemd unit reads them from `/etc/ochq/ochqd.env`.

| Flag | Env (unit) | Default | Description |
|---|---|---|---|
| `-tunnel-addr` | `OCHQD_TUNNEL_ADDR` | `:9000` | Where clients connect (TLS) |
| `-http-addr` | `OCHQD_HTTP_ADDR` | `:8080` | Public HTTP, normally behind the proxy |
| `-domain` | `OCHQD_DOMAIN` | `localhost` | Tunnels are served at `<name>.<domain>` |
| `-public-scheme` | `OCHQD_PUBLIC_SCHEME` | `http` | Scheme of URLs shown to clients |
| `-public-port` | `OCHQD_PUBLIC_PORT` | `8080` | Port in those URLs; `0` omits it |
| `-token` | `OCHQD_TOKEN` | — | Require this token from clients |
| `-tls-cert`, `-tls-key` | `OCHQD_TLS_CERT`, `OCHQD_TLS_KEY` | — | CA-signed certificate for :9000 |
| `-tls-dir` | `OCHQD_TLS_DIR` | — | Keep a self-signed certificate here |

A typical `/etc/ochq/ochqd.env` behind a proxy on 443:

```bash
OCHQD_TUNNEL_ADDR=:9000
OCHQD_HTTP_ADDR=127.0.0.1:8080      # proxy on the host; a proxy in Docker cannot
                                    # reach 127.0.0.1 — keep :8080 and firewall it (step 6)
OCHQD_DOMAIN=example.com
OCHQD_PUBLIC_SCHEME=https
OCHQD_PUBLIC_PORT=0
OCHQD_TOKEN=<openssl rand -hex 32>
```

**Always set a token** on a server reachable from the internet. Without one,
anybody can publish content under your domain and IP — phishing pages there
end up on blocklists, which also hurts any mail sent from that IP.

## 4. Tunnel TLS and the fingerprint

The tunnel port is always TLS 1.3. Either give `ochqd` a certificate for
`tunnel.example.com` (`-tls-cert`/`-tls-key`), or let it create a self-signed
one in `-tls-dir` (the unit uses `/var/lib/ochq/tls`). In the second case
clients pin its key; read the fingerprint from the log and hand it out with
the token:

```bash
journalctl -u ochqd | grep fingerprint
# ... msg="tunnel TLS ready" fingerprint=SHA256:zj3vhql3...
```

The fingerprint survives restarts and redeploys because the key is kept in
`/var/lib/ochq/tls`. Back that directory up; losing it means every client must
update `OCHQ_FINGERPRINT`.

## 5. Public HTTPS: the reverse proxy

`ochqd` speaks plain HTTP on `-http-addr` and expects a proxy in front to
terminate TLS for `*.example.com`. A wildcard certificate needs the ACME
**DNS-01** challenge, so pick a proxy and DNS provider combination that
supports it.

**Traefik** (file provider, Cloudflare DNS):

```yaml
# static
certificatesResolvers:
  le-dns:
    acme:
      storage: /data/acme.json
      dnsChallenge:
        provider: cloudflare        # token in CF_DNS_API_TOKEN

# dynamic
http:
  routers:
    ochq:
      rule: "HostRegexp(`^[a-z0-9-]+\\.example\\.com$`)"
      entryPoints: [websecure]
      service: ochq
      tls:
        certResolver: le-dns
        domains:
          - main: example.com
            sans: ["*.example.com"]
  services:
    ochq:
      loadBalancer:
        servers: [{ url: "http://127.0.0.1:8080" }]
```

**nginx** (certificate obtained separately, e.g. with certbot's DNS plugin):

```nginx
server {
    listen 443 ssl;
    server_name *.example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header Upgrade $http_upgrade;     # WebSockets
        proxy_set_header Connection $http_connection;
        proxy_buffering off;                        # streaming responses
    }
}
```

Without a proxy you can serve tunnels directly on `-http-addr` (plain HTTP,
URLs with a port) — fine for testing, not for production.

## 6. Firewall

```bash
sudo ufw allow 9000/tcp comment 'ochq tunnels'
# 8080 stays closed to the internet; allow only the proxy, e.g. a container:
sudo ufw allow from 172.30.0.2 to any port 8080 proto tcp comment 'proxy -> ochqd'
```

Docker-published ports bypass ufw. If the proxy runs in Docker and publishes
443, that port is open regardless of ufw rules; check `docker ps` rather than
`ufw status` to see what is exposed.

## 7. Verify

```bash
systemctl status ochqd
journalctl -u ochqd -f            # "tunnel opened" / "tunnel closed" per client

# from your machine
export OCHQ_SERVER=tunnel.example.com:9000 OCHQ_TOKEN=... OCHQ_FINGERPRINT=SHA256:...
ochq 3000 --sub hello
curl -I https://hello.example.com/
```
