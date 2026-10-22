# OCHQ — Open Channels, Handle Queries

*"ochiq"* — open. A lightweight, open-source reverse tunnel that exposes a local
port (`localhost:3000`) to the public internet under a subdomain.

```bash
ochq 3000
# → https://x8k2.ochq.uz  →  http://localhost:3000
```

> **Status:** early development (Milestone 1 — core engine). See
> [PROJECT_SPEC.md](PROJECT_SPEC.md) for the architecture and roadmap.

## Development

```bash
go test ./...
```

## License

[MIT](LICENSE)
