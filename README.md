# OCHQ — Open Channels, Handle Queries

*"ochiq"* — open. A lightweight, open-source reverse tunnel that exposes a local
port (`localhost:3000`) to the public internet under a subdomain.

```bash
ochq 3000
# → https://x8k2.ochq.uz  →  http://localhost:3000
```

> **Status:** early development (Milestone 1 — core engine). See
> [PROJECT_SPEC.md](PROJECT_SPEC.md) for the architecture and roadmap.

## Try it locally

No VPS or domain needed: `*.localhost` always resolves to `127.0.0.1`.

```bash
# terminal 1 — server
go run ./cmd/ochqd

# terminal 2 — something to expose
python3 -m http.server 3000

# terminal 3 — client; pin the fingerprint ochqd printed at startup
go run ./cmd/ochq 3000 --sub demo --fingerprint SHA256:...
# Forwarding  http://demo.localhost:8080 -> http://127.0.0.1:3000

# terminal 4
curl http://demo.localhost:8080/
```

## Tunnel security

The client ↔ server connection (`:9000`) is TLS 1.3. Until the server has a
CA-signed certificate (`-tls-cert`/`-tls-key`, e.g. Let's Encrypt), ochqd
uses a self-signed one and clients **pin its key fingerprint**, like SSH
host keys:

```bash
# on the server
journalctl -u ochqd | grep fingerprint
# on the client
export OCHQ_FINGERPRINT=SHA256:...
```

A client without a pin refuses an untrusted certificate and prints the
fingerprint it saw, so you can compare it with the server's.

## Development

```bash
go test -race ./...
```

## License

[MIT](LICENSE)
