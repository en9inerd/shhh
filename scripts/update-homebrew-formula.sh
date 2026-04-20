#!/bin/bash
# Updates the Homebrew tap formula with a new version and SHA256 checksums.
# Reads binaries from a local dist directory - works in CI and locally.
#
# Usage: update-homebrew-formula.sh <version> <dist_dir> <tap_repo_url>
#
# Example (CI):
#   bash scripts/update-homebrew-formula.sh 1.2.3 dist \
#     "https://x-access-token:${TAP_REPO_TOKEN}@github.com/en9inerd/homebrew-tap.git"
#
# Example (local, after downloading release assets to ./dist):
#   bash scripts/update-homebrew-formula.sh 1.2.3 dist \
#     "https://x-access-token:${TAP_REPO_TOKEN}@github.com/en9inerd/homebrew-tap.git"
set -euo pipefail

VERSION="${1:?version required}"
DIST="${2:?dist dir required}"
TAP_REPO="${3:?tap repo URL required}"

sha_of() { shasum -a 256 "${DIST}/$1" | awk '{print $1}'; }

SHA_MACOS_ARM64=$(sha_of shhh-cli-darwin-arm64)
SHA_MACOS_AMD64=$(sha_of shhh-cli-darwin-amd64)
SHA_LINUX_ARM64=$(sha_of shhh-cli-linux-arm64)
SHA_LINUX_AMD64=$(sha_of shhh-cli-linux-amd64)

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

git clone "$TAP_REPO" "$TMPDIR/tap"
cp packaging/homebrew/shhh-cli.rb "$TMPDIR/tap/Formula/shhh-cli.rb"

sed -i.bak \
  -e "s/VERSION_PLACEHOLDER/${VERSION}/g" \
  -e "s/SHA256_MACOS_ARM64/${SHA_MACOS_ARM64}/g" \
  -e "s/SHA256_MACOS_AMD64/${SHA_MACOS_AMD64}/g" \
  -e "s/SHA256_LINUX_ARM64/${SHA_LINUX_ARM64}/g" \
  -e "s/SHA256_LINUX_AMD64/${SHA_LINUX_AMD64}/g" \
  "$TMPDIR/tap/Formula/shhh-cli.rb"
rm -f "$TMPDIR/tap/Formula/shhh-cli.rb.bak"

cd "$TMPDIR/tap"
git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git add Formula/shhh-cli.rb
git commit -m "shhh-cli ${VERSION}" || true
git push

echo "Updated homebrew-tap Formula/shhh-cli.rb to v${VERSION}"
