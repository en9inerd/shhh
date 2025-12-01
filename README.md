# SHHH

Service for easy way to transfer credentials

> **Current Status**: "Under development"

## Features

- Encrypted secret storage with AES-256-GCM
- Automatic expiration
- One-time retrieval (secrets deleted after access)
- Support for text and file uploads
- Docker-ready with nginx reverse proxy
- SSL/TLS support
- Security headers and rate limiting

## Quick Start

### Docker (Recommended)

```bash
# 1. Copy environment file
cp .env.example .env

# 2. Start services
docker-compose up -d

# 3. View logs
docker-compose logs -f
```

The application will be available at `https://localhost` (or your configured domain).

### Local Development

```bash
# Build
go build -o dist/shhh ./cmd/shhh/

# Run
./dist/shhh

# Or with custom port
./dist/shhh -port 8080
```

## Configuration

All configuration is done via environment variables. See `.env.example` for available options:

- `SHHH_PORT` - Application port (default: 8000)
- `SHHH_MIN_PHRASE_SIZE` - Min passphrase length (default: 5)
- `SHHH_MAX_PHRASE_SIZE` - Max passphrase length (default: 128)
- `SHHH_MAX_ITEMS` - Max items in memory (default: 100)
- `SHHH_MAX_FILE_SIZE` - Max file size in bytes (default: 2MB)
- `SHHH_MAX_RETENTION` - Max retention time (default: 24h)
- `NGINX_SERVER_NAME` - Server name for nginx (default: localhost)
- `NGINX_SSL_ENABLED` - Enable SSL/TLS (default: false)

## SSL Setup

### Development (Self-signed Certificate)

```bash
mkdir -p ssl
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout ssl/key.pem \
  -out ssl/cert.pem \
  -subj "/CN=localhost"
```

Then set `NGINX_SSL_ENABLED=true` in `.env` and restart:

```bash
docker-compose restart app
```

### Production (Let's Encrypt)

1. **Obtain certificate on host:**
   ```bash
   sudo certbot certonly --standalone -d your-domain.com
   ```

2. **Mount certificates in docker-compose.yml:**
   ```yaml
   volumes:
     - /etc/letsencrypt/live/your-domain.com/fullchain.pem:/etc/nginx/ssl/cert.pem:ro
     - /etc/letsencrypt/live/your-domain.com/privkey.pem:/etc/nginx/ssl/key.pem:ro
   ```
   Or copy to local ssl directory:
   ```bash
   mkdir -p ssl
   sudo cp /etc/letsencrypt/live/your-domain.com/fullchain.pem ssl/cert.pem
   sudo cp /etc/letsencrypt/live/your-domain.com/privkey.pem ssl/key.pem
   ```

3. **Set environment variables:**
   ```bash
   NGINX_SSL_ENABLED=true
   NGINX_SERVER_NAME=your-domain.com
   ```

## API Endpoints

### Create Text Secret
```bash
POST /api/secret
Content-Type: application/json

{
  "secret": "my secret text",
  "passphrase": "mypass",
  "exp": 3600
}
```

### Create File Secret
```bash
POST /api/file
Content-Type: multipart/form-data

file: <file>
passphrase: mypass
exp: 3600
```

### Retrieve Secret
```bash
POST /api/secret/{id}
Content-Type: application/json

{
  "passphrase": "mypass"
}
```

### Get Parameters
```bash
GET /api/params
```

## Web Interface

The application includes a web interface:

- `GET /` - Create secret page
- `GET /secret/{id}` - Retrieve secret page
- `POST /web/secret` - Create text secret (web)
- `POST /web/file` - Create file secret (web)
- `POST /web/retrieve` - Retrieve secret (web)

## Security Features

### Application Security
- AES-256-GCM encryption with Argon2 key derivation
- One-time retrieval (secrets deleted after access)
- Automatic expiration and cleanup
- Memory-only storage (no filesystem writes)
- Template auto-escaping (XSS protection)
- Input validation and size limits

### Nginx Security (Docker)
- Per-IP rate limiting (API: 10r/s, Web: 20r/s)
- Request size limits (2.5MB max)
- Security headers (HSTS, X-Frame-Options, etc.)
- SSL/TLS support with modern ciphers
- CORS support with configurable origins

## Docker Commands

```bash
# Start services
docker-compose up -d

# Stop services
docker-compose down

# View logs
docker-compose logs -f

# Restart services
docker-compose restart

# Check status
docker-compose ps

# Rebuild images
docker-compose build
```

## Troubleshooting

### Container won't start
```bash
# Check logs
docker-compose logs app

# Test nginx configuration
docker-compose exec app nginx -t
```

### SSL certificate issues
```bash
# Verify certificates exist
ls -la ssl/

# Check certificate validity
openssl x509 -in ssl/cert.pem -text -noout
```

### Can't connect to backend
```bash
# Verify app is running
docker-compose ps app

# Test health endpoint
curl https://localhost/health

# Check app logs
docker-compose logs app
```

## Development

### Building
```bash
make build
```

### Running Tests
```bash
go test ./...
```

### Project Structure
```
.
├── cmd/shhh/          # Application entry point
├── internal/
│   ├── config/        # Configuration
│   ├── crypto/        # Encryption service
│   ├── memstore/      # Memory storage
│   ├── server/        # HTTP server and handlers
│   └── validator/     # Input validation
├── ui/                # Web UI (templates, static files)
├── Dockerfile         # App container
├── docker-compose.yml # Docker Compose configuration
├── nginx.conf         # Nginx configuration
└── ssl/               # SSL certificates (created at runtime)
```

## License

See LICENSE file for details.
