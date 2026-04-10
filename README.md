# SHHH

[![Docker Hub](https://img.shields.io/docker/v/enginerd/shhh?label=Docker%20Hub&logo=docker&sort=semver)](https://hub.docker.com/r/enginerd/shhh)

A simple service for sharing secrets securely. Encrypt your text or files, share a link, and they'll self-destruct after being retrieved once.

Inspired by [umputun/secrets](https://github.com/umputun/secrets/).

> **Threat-model note:** Plaintext exists in the app's memory (and travels over the wire in TLS when you use HTTPS in front of the service). This is standard for a hosted "paste bin"–style tool; it is **not** end-to-end encryption in the browser.

## Features

- AES-256-GCM encryption with Argon2id key derivation
- One-time retrieval (secrets are deleted after being accessed)
- Automatic expiration and cleanup
- Text and file uploads (up to 2MB by default)
- Web UI built with HTMX (no JavaScript framework needed)
- Single binary, single container — no sidecar reverse proxy required

## Quick Start

### Docker

```bash
cp .env.example .env
docker-compose up -d
```

The app will be available at `http://localhost:8000`.

### Local Development

```bash
go build -o dist/shhh ./cmd/shhh/
./dist/shhh
```

Or use the Makefile:

```bash
make run
```

## Configuration

All settings are controlled via environment variables. Check `.env.example` for the full list:

| Variable | Default | Description |
|---|---|---|
| `SHHH_PORT` | `8000` | Port the app listens on |
| `SHHH_BASE_URL` | _(empty)_ | Public base URL for generated links (e.g. `https://shhh.example.com`). When unset, derived from the request. |
| `SHHH_CORS_ORIGIN` | `*` | Allowed CORS origin. Restrict this in production. |
| `SHHH_MIN_PHRASE_SIZE` | `5` | Minimum passphrase length |
| `SHHH_MAX_PHRASE_SIZE` | `128` | Maximum passphrase length |
| `SHHH_MAX_ITEMS` | `100` | Max number of secrets in memory |
| `SHHH_MAX_FILE_SIZE` | `2097152` | Max file size in bytes (default 2 MB) |
| `SHHH_MAX_RETENTION` | `24h` | Maximum time a secret can live |
| `SHHH_VERBOSE` | `false` | Enable debug logging |

## Reverse Proxy (optional)

The Go binary handles everything — TLS termination is the only reason you'd add a reverse proxy. [Caddy](https://caddyserver.com/) is recommended for automatic HTTPS:

```
shhh.example.com {
    reverse_proxy localhost:8000
}
```

Caddy gives you automatic Let's Encrypt certificates, HTTP/2, HTTP/3, and HTTP-to-HTTPS redirects with zero extra configuration.

## Production checklist

| Topic | Guidance |
|--------|----------|
| **TLS** | Do not expose the Go listener directly on the public internet. Terminate TLS with Caddy, nginx, or Traefik (see above). |
| **Public URL** | Set `SHHH_BASE_URL` to your canonical HTTPS origin so shared links match what users expect. If unset, the app infers the scheme from each request (fine behind a proxy that sets `X-Forwarded-Proto`). |
| **CORS** | Default `SHHH_CORS_ORIGIN=*` is common for tools without cookies. If only a specific browser origin should call the API, set it to that origin. |
| **Logs** | Ship stdout/stderr to your log stack. Use `SHHH_VERBOSE=true` only when debugging. |
| **Capacity** | Argon2 work per create is CPU- and memory-heavy; size CPU/RAM and `SHHH_MAX_ITEMS` / rate limits for your load. |
| **High availability** | The store is **in-process memory only**. One replica is the supported model; multiple replicas without shared storage would split the map and break lookups. Restarts drop all secrets. |

## API

### Create a text secret

```bash
POST /api/secret
Content-Type: application/json

{
  "secret": "my secret text",
  "passphrase": "mypass",
  "exp": 3600
}
```

`201 Created` response body:

```json
{
  "key": "abc123...",
  "expires_at": "2026-04-09T12:34:56Z"
}
```

- Request field **`exp`** is the lifetime in **seconds** (minimum 1). It must not exceed **`SHHH_MAX_RETENTION`**; otherwise the API returns **`400`** with a validation error (same rule for file uploads and the web UI).
- **`expires_at`** is the UTC expiry time (RFC 3339) computed from the accepted `exp`.

### Create a file secret

```bash
POST /api/file
Content-Type: multipart/form-data

file: <file>
passphrase: mypass
exp: 3600
```

`201 Created` response body:

```json
{
  "key": "abc123...",
  "filename": "document.pdf",
  "expires_at": "2026-04-09T12:34:56Z"
}
```

### Retrieve a secret

```bash
POST /api/secret/{id}
Content-Type: application/json

{
  "passphrase": "mypass"
}
```

Returns the decrypted secret. The secret is deleted immediately after retrieval.

### Get configuration parameters

```bash
GET /api/params
```

Returns the current limits and settings (useful for validating passphrase length and payload size before calling the create endpoints).

## Security

- **Encryption**: AES-256-GCM with Argon2id key derivation (32MB memory, 2 iterations, 1 thread), performed **on the server** before storing ciphertext
- **Storage**: Everything is in-memory only. Nothing is written to disk.
- **Input validation**: All inputs are validated and sanitized (including filename sanitization against header injection).
- **Security headers**: CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy, Cross-Origin-Opener-Policy.
- **Rate limiting**: Per-IP token bucket — 10 req/s (burst 20) for the API, 20 req/s (burst 30) for the web UI.

## Development

```bash
make build            # Build
make test             # Run tests
make run              # Run locally (sources .env)
make run-verbose      # Run with debug logging
make format-html      # Format HTML templates
make update-htmx      # Update HTMX
```

Project structure:

```
.
├── cmd/shhh/          # Main entry point
├── internal/
│   ├── config/        # Config parsing
│   ├── crypto/        # Encryption (AES + Argon2id)
│   ├── log/           # Logging
│   ├── memstore/      # In-memory storage
│   └── server/        # HTTP handlers and routes
├── ui/                # Web UI (templates + static files)
├── Dockerfile
└── docker-compose.yml
```

## License

See LICENSE file.
