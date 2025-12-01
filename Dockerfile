# Multi-stage build for SHHH application with nginx
# Build from root directory: docker build -t shhh .

# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /bin/shhh \
    ./cmd/shhh/

# Stage 2: Runtime with nginx
FROM alpine:latest

RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    nginx \
    gettext \
    su-exec \
    curl \
    && update-ca-certificates

# Create non-root user
RUN addgroup -g 1000 shhh && \
    adduser -D -u 1000 -G shhh shhh

WORKDIR /app

# Copy binary from builder
COPY --from=builder /bin/shhh /app/shhh

# Copy nginx config
COPY nginx.conf /etc/nginx/nginx.conf.template

# Create entrypoint script
RUN cat > /entrypoint.sh << 'EOF'
#!/bin/sh
set -e

# Set default CORS origin if not provided
export NGINX_CORS_ORIGIN="${NGINX_CORS_ORIGIN:-*}"

# Substitute environment variables in nginx config
envsubst '${NGINX_BACKEND} ${NGINX_SERVER_NAME} ${NGINX_CORS_ORIGIN}' < /etc/nginx/nginx.conf.template > /etc/nginx/nginx.conf

# Remove SSL block if SSL is disabled or certificates are missing
if [ "${NGINX_SSL_ENABLED}" != "true" ] || [ ! -f /etc/nginx/ssl/cert.pem ] || [ ! -f /etc/nginx/ssl/key.pem ]; then
  sed -i '/listen 443/,/^    }/d' /etc/nginx/nginx.conf
fi

# Test nginx configuration
nginx -t

# Start nginx in background
nginx

# Run application as non-root user
exec su-exec shhh /app/shhh
EOF

# Set permissions and create directories
RUN mkdir -p /etc/nginx/ssl /var/log/nginx && \
    chown -R shhh:shhh /app /var/log/nginx && \
    chown -R nginx:nginx /etc/nginx && \
    chmod +x /entrypoint.sh

EXPOSE 80 443 8000

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
