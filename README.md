# SHHH

[![Docker Hub](https://img.shields.io/docker/v/enginerd/shhh?label=Docker%20Hub&logo=docker&sort=semver)](https://hub.docker.com/r/enginerd/shhh)

Two tools in one:

- **Secrets** — share a text or file, encrypted once, retrieved once, then gone.
- **Channels** — end-to-end encrypted broadcast channels for real-time messaging between trusted devices.

Inspired by [umputun/secrets](https://github.com/umputun/secrets/).

## Threat model

| | Secrets | Channels |
|---|---|---|
| **Encryption** | Server-side (AES-256-GCM + Argon2id). Plaintext passes through server memory. | Client-side E2E (PBKDF2-SHA256 + AES-256-GCM). Server stores and forwards opaque blobs only — never sees plaintext. |
| **Passphrase** | Sent to the server to decrypt. | Never leaves the client. |
| **Storage** | In-memory, deleted on retrieval. | In-memory queue, TTL-based expiry. |

## Features

- One-time secret sharing — text and files up to 2 MB, self-destruct on retrieval
- E2E encrypted channels — real-time SSE broadcast, passphrase stays in the browser/CLI
- Web UI built with HTMX (no JavaScript framework)
- CLI (`shhh-cli`) for channel push, pull, and interactive watch
- Single binary, single container — no sidecar reverse proxy required

## Quick Start

### Docker

```bash
cp .env.example .env
docker-compose up -d
```

### Local

```bash
go build -o dist/shhh ./cmd/shhh
./dist/shhh
```

## CLI (`shhh-cli`)

### Install

```bash
brew tap en9inerd/tap
brew install shhh-cli
```

Or build from source:

```bash
go build -o shhh-cli ./cmd/shhh-cli
```

### Usage

```
shhh-cli [--server URL] [--passphrase PHRASE] [--name DEVICE_NAME] <command> <uuid>

push <uuid> [text]       Encrypt text and push to channel (reads stdin if omitted)
push <uuid> --file PATH  Encrypt file and push to channel
pull <uuid>              Fetch and decrypt all queued messages
watch <uuid>             Connect SSE; receive messages and send interactively
```

**Environment variables** (alternatives to flags):

| Variable | Flag equivalent |
|---|---|
| `SHHH_SERVER` | `--server` |
| `SHHH_PASSPHRASE` | `--passphrase` |
| `SHHH_DEVICE_NAME` | `--name` |

**Interactive watch mode** — once connected, type a message and press Enter to send. Use `:file /path/to/file` to send a file. Press Ctrl+C to stop.

The passphrase is never sent to the server. All encryption and decryption runs locally.

## Configuration

All settings via environment variables:

### Server

| Variable | Default | Description |
|---|---|---|
| `SHHH_PORT` | `8000` | Port the server listens on |
| `SHHH_BASE_URL` | _(empty)_ | Public base URL for generated links (e.g. `https://shhh.example.com`). Inferred from request when unset. |
| `SHHH_CORS_ORIGIN` | _(empty)_ | Allowed CORS origin. Empty = same-origin only. |
| `SHHH_MIN_PHRASE_SIZE` | `5` | Minimum passphrase length |
| `SHHH_MAX_PHRASE_SIZE` | `128` | Maximum passphrase length |
| `SHHH_MAX_ITEMS` | `100` | Max secrets in memory |
| `SHHH_MAX_FILE_SIZE` | `2097152` | Max file size in bytes (2 MB) |
| `SHHH_MAX_RETENTION` | `24h` | Max secret lifetime |
| `SHHH_VERBOSE` | `false` | Enable debug logging |
| `SHHH_TRUSTED_PROXIES` | _(empty)_ | Comma-separated list of trusted proxy IPs/CIDRs for `X-Forwarded-For` |

### Channels

| Variable | Default | Description |
|---|---|---|
| `SHHH_CHANNELS` | _(empty)_ | Comma-separated list of channel UUIDs to enable (32-char lowercase hex each) |
| `SHHH_CHANNEL_MSG_TTL` | `24h` | How long messages stay in the queue (falls back to `SHHH_MAX_RETENTION` if zero) |
| `SHHH_CHANNEL_MAX_MSGS` | `20` | Max queued messages per channel |
| `SHHH_CHANNEL_MAX_WATCHERS` | `10` | Max concurrent SSE watchers per channel |
| `SHHH_WATCH_CONN_PER_IP` | `3` | Max concurrent watch connections per IP |
| `SHHH_WATCH_RPS_PER_IP` | `2` | Watch endpoint rate limit (requests/sec per IP) |

Channels must be pre-configured by the admin — clients cannot create them dynamically. Generate a UUID and add it to `SHHH_CHANNELS`:

```bash
python3 -c "import secrets; print(secrets.token_hex(16))"
```

## Reverse Proxy (optional)

TLS termination is the only reason to add a reverse proxy. [Caddy](https://caddyserver.com/) is recommended:

```
shhh.example.com {
    reverse_proxy localhost:8000
}
```

## Production Checklist

| Topic | Guidance |
|---|---|
| **TLS** | Terminate TLS with Caddy, nginx, or Traefik. Do not expose the Go listener directly. |
| **Public URL** | Set `SHHH_BASE_URL` so shared links match your canonical HTTPS origin. |
| **CORS** | Disabled by default (same-origin only). Set `SHHH_CORS_ORIGIN` only if the API is called cross-origin. |
| **Trusted proxies** | Set `SHHH_TRUSTED_PROXIES` to your proxy IPs/CIDRs so `X-Forwarded-For` is trusted only from known sources. |
| **Logs** | Ship stdout/stderr to your log stack. Use `SHHH_VERBOSE=true` only when debugging. Secret and file content is never logged. |
| **Capacity** | Argon2 work per secret creation is CPU- and memory-heavy. Size CPU/RAM and tune `SHHH_MAX_ITEMS` and rate limits for your load. |
| **High availability** | The store is in-process memory only. One replica is the supported model; multiple replicas without shared storage split the map and break lookups. Restarts drop all secrets and queued channel messages. |

## API

### Secrets

#### Create a text secret

```
POST /api/secret
Content-Type: application/json

{"secret": "my secret", "passphrase": "mypass", "exp": 3600}
```

`201` response:

```json
{"key": "abc123...", "expires_at": "2026-04-09T12:34:56Z"}
```

`exp` is lifetime in seconds (minimum 1, maximum `SHHH_MAX_RETENTION`).

#### Create a file secret

```
POST /api/file
Content-Type: multipart/form-data

file=<file>, passphrase=mypass, exp=3600
```

`201` response:

```json
{"key": "abc123...", "filename": "document.pdf", "expires_at": "2026-04-09T12:34:56Z"}
```

#### Retrieve a secret

```
POST /api/secret/{id}
Content-Type: application/json

{"passphrase": "mypass"}
```

Returns the decrypted secret. Deleted immediately after retrieval.

#### Get server parameters

```
GET /api/params
```

Returns current limits (passphrase size, file size, retention) — useful for client-side validation.

### Channels

All channel data is opaque to the server. Encryption and decryption happen entirely on the client.

#### Push a message

```
PUT /api/channel/{uuid}
Content-Type: application/octet-stream

<encrypted binary blob>
```

`204` on success. `429` if the queue is full.

#### Pull queued messages

```
GET /api/channel/{uuid}[?limit=N]
```

`200` response:

```json
{
  "messages": [
    {"blob": "<base64>", "pushed_at": "2026-04-09T12:34:56Z"}
  ]
}
```

#### Watch (SSE)

```
GET /api/channel/{uuid}/watch
Accept: text/event-stream
```

Server-sent events stream. Sends a `connected` event on open with a snapshot of queued messages, then a `message` event for each new push. Keepalive comments (`: keepalive`) are sent every 15 seconds.

## Security

**Secrets**
- AES-256-GCM encryption with Argon2id key derivation (32 MB memory, 2 iterations, 1 thread), performed server-side before storage.
- Passphrase is used for decryption and never persisted.

**Channels**
- PBKDF2-SHA256 (600 000 iterations) key derivation + AES-256-GCM, performed entirely client-side.
- The channel UUID is used as authenticated additional data (AAD) — blobs from one channel cannot be replayed into another.
- The server stores and forwards ciphertext blobs only.

**Both**
- All secrets and channel messages stored in-memory only — nothing written to disk.
- Input validation and filename sanitization on all endpoints.
- Security headers: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy, Cross-Origin-Opener-Policy.
- Rate limiting: per-IP token bucket (10 req/s, burst 20 for the API; 20 req/s, burst 30 for the web UI; separate tighter limits for the SSE watch endpoint).

## Development

```bash
make build            # Build server binary
make test             # Run tests with race detector
make run              # Run locally (sources .env)
make run-verbose      # Run with debug logging
make format-html      # Format HTML templates
make update-htmx      # Update HTMX
```


Project structure:

```
.
├── cmd/
│   ├── shhh/          # Server entry point
│   └── shhh-cli/      # CLI entry point
├── internal/
│   ├── channel/       # Channel store, E2E crypto (PBKDF2 + AES-GCM)
│   ├── config/        # Config parsing
│   ├── crypto/        # Server-side crypto (Argon2id + AES-GCM)
│   ├── log/           # Logging
│   ├── memstore/      # In-memory secret storage
│   ├── server/        # HTTP handlers and routes
│   └── util/          # Shared utilities
├── ui/                # Web UI (templates + static assets)
├── scripts/           # CI helper scripts (Homebrew formula update)
├── packaging/
│   └── homebrew/      # Homebrew formula template
├── Dockerfile
└── docker-compose.yml
```

## License

See LICENSE file.
