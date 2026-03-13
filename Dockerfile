# ---------- Build ----------
# Pin builder to the build machine's native platform so Go cross-compiles
# natively instead of running under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.26.1-alpine AS builder

RUN apk update && apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
      -gcflags="all=-l -B" \
      -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o /shhh \
      ./cmd/shhh

# ---------- Runtime ----------
FROM nginx:1.28-alpine

RUN apk update && \
    apk add --no-cache \
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
