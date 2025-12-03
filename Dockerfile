# ---------- Build ----------
FROM golang:1.25-alpine AS builder

# Install packages with --no-scripts to avoid trigger errors in QEMU emulation
RUN apk update && apk add --no-cache --no-scripts git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /shhh \
      ./cmd/shhh

# ---------- Runtime ----------
FROM nginx:alpine

# Install packages
# Use --no-scripts to skip triggers that fail in QEMU emulation for multi-arch builds
# The packages function correctly without their post-install triggers
RUN apk update && \
    apk add --no-cache --no-scripts \
        ca-certificates \
        tzdata \
        gettext \
        su-exec \
        curl

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app

COPY --from=builder /shhh /app/shhh
COPY nginx.conf /etc/nginx/nginx.conf.template
COPY nginx-ssl.conf /etc/nginx/nginx-ssl.conf.template

COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 80 443 8000

HEALTHCHECK CMD curl -f http://localhost:8000/health || exit 1

ENTRYPOINT ["/entrypoint.sh"]
