```markdown
# Project Specification: OCHQ (Open Channels, Handle Queries)

## 1. Project Overview & Vision
**OCHQ** (derived from the Uzbek word *"ochiq"*, meaning *"open"*) is a high-performance, lightweight, open-source reverse-tunneling platform designed to expose local development ports (`localhost:PORT`) to the public internet under custom or dynamic subdomains.

The initial phase focuses on a robust, idiomatic **Go** implementation of:
1. `ochq` (Client CLI tool)
2. `ochqd` (Server Daemon running on a Linux VPS)

Future phases will introduce an integrated Web UI dashboard (similar to ngrok's local inspection dashboard at `localhost:4040`).

---

## 2. Technical References & Inspiration
The developer should inspect and adapt proven patterns from:
- **`jprq` (azimjohn/jprq):** Lightweight tunnel logic, simple control connection, subdomain registration, and minimal footprint.
- **`ngrok`:** Multi-stream multiplexing, connection recovery, CLI UI (ANSI escape codes, clean terminal dashboard), and developer ergonomics.
- **`hashicorp/yamux`:** Multiplexing library for establishing multiple parallel bidirectional virtual streams over a single physical TCP connection (preventing Head-of-Line blocking).

---

## 3. High-Level Architecture & Flow

```text
[External User / Browser]
          │
          ▼  HTTPS Request (e.g. [https://x8k2.ochq.uz/api/v1](https://x8k2.ochq.uz/api/v1))
[Wildcard DNS (*.ochq.uz)] -> [VPS Server Public IP]
          │
          ▼
[Reverse Proxy / TLS Terminator (Caddy or Nginx with Wildcard SSL)]
          │  HTTP Forward (Proxy to internal port :8080)
          ▼
[ochqd (Server Daemon)]
          │  Inspects HTTP Host header ("x8k2.ochq.uz")
          │  Looks up active Yamux session for subdomain "x8k2"
          │  Opens a new Yamux Stream inside the established TCP tunnel
          ▼
[TCP / TLS Connection (:9000)]
          ▲
          │  Persistent Yamux Multiplexed Tunnel
[ochq (Client CLI)]
          │  Receives incoming Yamux Stream
          │  Establishes local connection (net.Dial)
          ▼
[Target Local Service (e.g., http://localhost:3000)]

```

---

## 4. Key Functional Components

### A. Server Daemon (`cmd/ochqd`)

* **Control & Tunnel Listener (`:9000`):**
* Accepts incoming TCP connections from `ochq` clients.
* Manages handshakes, auth tokens (optional), and subdomain allocation (random 4-character hex or requested custom aliases).
* Initializes and maintains a thread-safe registry of `yamux.Session` mapped by `subdomain`.
* Implements ping/pong heartbeat to detect dead or stale client connections immediately.


* **Public HTTP Ingress (`:8080`):**
* Receives HTTP traffic from Caddy/Nginx.
* Extracts the routing key from `r.Host` (e.g., `x8k2` from `x8k2.ochq.uz`).
* Fetches the client's `yamux.Session`, opens a virtual stream, forwards the raw HTTP request, reads the HTTP response, and writes it back to the caller.



### B. Client CLI (`cmd/ochq`)

* **Invocation Syntax:**
```bash
ochq 3000
ochq http 8080
ochq http 3000 --sub my-api --server tunnel.ochq.uz:9000

```


* **Connection Logic:**
* Dials the remote VPS server (`:9000`).
* Performs handshake (requests port, sends preferred subdomain if specified).
* Initializes `yamux.Client(conn)`.


* **Stream Dispatcher:**
* Runs a persistent loop accepting `session.Accept()` streams.
* Spawns a dedicated goroutine for each stream to handle bidirectional I/O (`io.Copy`) between the remote stream and the local `127.0.0.1:PORT`.


* **Terminal UI / UX:**
* Renders a clean ASCII/Unicode banner:
```text
       _         
 ___ _| |_ ___ ___ 
| . |  _|   | . |
|___|___|_|_|_  |  v1.0.0
              |_|  Open Channels, Handle Queries

[✓] Status:       Online
[✓] Forwarding:   http://localhost:3000
[✓] Public URL:   [https://x8k2.ochq.uz](https://x8k2.ochq.uz)

Logs:
GET  /api/v1/health  200 OK  (14ms)
POST /auth/login     200 OK  (42ms)

```


* Graceful shutdown on `SIGINT` / `SIGTERM` (closes tunnel and cleans up server-side state).



---

## 5. Repository Structure

```text
ochq/
├── cmd/
│   ├── ochq/              # Client CLI entrypoint
│   │   └── main.go
│   └── ochqd/             # Server daemon entrypoint
│       └── main.go
├── internal/
│   ├── proto/             # Protocol definitions, wire messages & handshakes
│   │   └── messages.go
│   ├── server/            # ochqd routing, session manager, HTTP handler
│   │   ├── registry.go
│   │   ├── handler.go
│   │   └── server.go
│   ├── client/            # ochq connection, proxy forwarder, stream runner
│   │   └── runner.go
│   └── ui/                # Terminal dashboard, color logs, status renderer
│       └── printer.go
├── pkg/
│   └── version/           # Version info and build tags
│       └── version.go
├── scripts/
│   ├── install.sh         # One-line client installer
│   └── setup-vps.sh       # Systemd and Caddy automation for server
├── Caddyfile.example      # Sample reverse proxy config with Wildcard SSL
├── Dockerfile             # Containerized build for ochqd
├── go.mod
├── go.sum
└── README.md

```

---

## 6. Development Roadmap & Priorities

1. **Milestone 1 (Core Engine):**
* Implement `internal/proto` handshake messages (JSON or lightweight binary).
* Implement basic `ochqd` TCP listener and `ochq` client using `github.com/hashicorp/yamux`.
* Verify bidirectional HTTP proxying through local loopback.


2. **Milestone 2 (Server Routing & Wildcards):**
* Subdomain registry (`map[string]*yamux.Session` with `sync.RWMutex`).
* Dynamic 4-character ID generation (`crypto/rand`).
* HTTP Host header dispatching.


3. **Milestone 3 (CLI Polish & Reliability):**
* Live request logging in terminal (Status Code, Method, Path, Latency).
* Reconnect backoff strategy when the network drops.


4. **Milestone 4 (Future - Web Inspection UI):**
* Embed a local web server (e.g. `localhost:4040`) allowing inspection of incoming headers, query parameters, payloads, and replay requests.



```

