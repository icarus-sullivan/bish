#!/bin/sh
# Downloads the pinned cloudflared release binaries used to bundle a
# Cloudflare Quick Tunnel with the app (internal/liveshare's off-LAN share
# links). Run by `make build`/`make darwin`/`make init` before `wails build`,
# since the Go side go:embeds whatever lands in
# internal/liveshare/assets/cloudflared/ — a platform with no binary there
# just falls back to a LAN-only share link (see internal/liveshare/cloudflared.go),
# so it's safe to skip this in offline dev.
set -e

VERSION=2026.9.1
OUT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)/internal/liveshare/assets/cloudflared"
GOOS="${1:-$(go env GOOS)}"
mkdir -p "$OUT"

fetch_raw() {
  # $1 = release asset name, $2 = output filename
  dest="$OUT/$2"
  if [ -f "$dest" ] && [ -z "$FORCE" ]; then
    echo "cloudflared: $2 already present, skip"
    return
  fi
  url="https://github.com/cloudflare/cloudflared/releases/download/$VERSION/$1"
  echo "cloudflared: fetching $1"
  curl -fL --progress-bar -o "$dest" "$url"
  chmod +x "$dest"
}

fetch_tgz() {
  # $1 = release asset name (.tgz containing a single `cloudflared` binary), $2 = output filename
  dest="$OUT/$2"
  if [ -f "$dest" ] && [ -z "$FORCE" ]; then
    echo "cloudflared: $2 already present, skip"
    return
  fi
  url="https://github.com/cloudflare/cloudflared/releases/download/$VERSION/$1"
  tmp="$(mktemp -d)"
  echo "cloudflared: fetching $1"
  curl -fL --progress-bar -o "$tmp/a.tgz" "$url"
  tar -xzf "$tmp/a.tgz" -C "$tmp"
  mv "$tmp/cloudflared" "$dest"
  chmod +x "$dest"
  rm -rf "$tmp"
}

case "$GOOS" in
  darwin)
    # both arches always — `wails build -platform darwin/universal` compiles
    # for amd64 and arm64 regardless of host arch
    fetch_tgz cloudflared-darwin-amd64.tgz cloudflared-darwin-amd64
    fetch_tgz cloudflared-darwin-arm64.tgz cloudflared-darwin-arm64
    ;;
  linux)
    arch="$(go env GOARCH)"
    fetch_raw "cloudflared-linux-$arch" "cloudflared-linux-$arch"
    ;;
  windows)
    fetch_raw cloudflared-windows-amd64.exe cloudflared-windows-amd64.exe
    ;;
  *)
    echo "cloudflared: unsupported GOOS '$GOOS', skipping bundle (share links will fall back to LAN-only)" >&2
    ;;
esac
