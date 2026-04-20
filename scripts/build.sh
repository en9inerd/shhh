#!/bin/bash
set -euo pipefail

CGO_ENABLED=0
DIST_DIR="${DIST_DIR:-dist}"
VERSION="${VERSION:-dev}"

platforms=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")

mkdir -p "$DIST_DIR"

for platform in "${platforms[@]}"; do
  GOOS="${platform%/*}"
  GOARCH="${platform#*/}"
  output="$DIST_DIR/shhh-cli-${GOOS}-${GOARCH}"

  echo "Building $output (version $VERSION)"
  CGO_ENABLED=$CGO_ENABLED GOOS=$GOOS GOARCH=$GOARCH \
    go build -gcflags="all=-l -B" -trimpath \
    -ldflags="-s -w -X main.version=$VERSION" \
    -o "$output" ./cmd/shhh-cli
done

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$DIST_DIR" && sha256sum shhh-cli-* > SHA256SUMS)
else
  (cd "$DIST_DIR" && shasum -a 256 shhh-cli-* > SHA256SUMS)
fi
echo "SHA256SUMS written"
