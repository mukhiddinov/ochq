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

# terminal 3 — client
go run ./cmd/ochq 3000 --sub demo
# Forwarding  http://demo.localhost:8080 -> http://127.0.0.1:3000

# terminal 4
curl http://demo.localhost:8080/
```

## Development

```bash
go test -race ./...
```

## License

[MIT](LICENSE)
