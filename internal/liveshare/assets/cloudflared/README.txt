Populated by scripts/fetch-cloudflared.sh at build time with cloudflared
binaries (named cloudflared-<GOOS>-<GOARCH>[.exe]) for the Cloudflare Quick
Tunnel used by internal/liveshare/cloudflared.go. Not checked into git.

This file exists so `go build` succeeds even before that script has run —
go:embed requires at least one non-hidden file to match in an embedded
directory. Missing the binary for the running platform just makes share
links fall back to LAN-only (see ErrCloudflaredUnavailable).
