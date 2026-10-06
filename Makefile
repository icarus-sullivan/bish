# `go env GOPATH` prints a backslash-separated path on Windows (e.g.
# C:\Users\runneradmin\go); fed straight into a recipe line, the shell
# (sh/bash under Git Bash) treats each backslash as an escape character and
# eats it, mangling the path (C:Usersrunneradmingo/bin/wails -> not found).
# Normalize to forward slashes so the same recipe works on every OS.
WAILS := $(shell go env GOPATH | tr '\\' '/')/bin/wails
UNAME_S := $(shell uname -s)
TAGS   ?=

VERSION  := $(shell scripts/read-config.sh version)
APP_NAME := $(shell scripts/read-config.sh app.name)
APP_DESC := $(shell scripts/read-config.sh app.description)
CLI_NAME := $(shell scripts/read-config.sh cli.name)
CLI_DESC := $(shell scripts/read-config.sh cli.description)

.PHONY: init dev build install darwin sync-config fetch-cloudflared fetch-vosk-model darwin-plist

init: fetch-cloudflared fetch-vosk-model
	go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
	go mod download
	cd frontend && pnpm install

# Bundles cloudflared (Quick Tunnel binary for off-LAN share links) into
# internal/liveshare/assets/cloudflared/ so the Go side's go:embed picks it
# up at compile time. Safe to skip/fail offline — liveshare falls back to
# LAN-only links when no binary is present for the running platform.
fetch-cloudflared:
	scripts/fetch-cloudflared.sh

# Bundles the Vosk speech model (voice dictation in the AI panel, run fully
# in the webview by vosk-browser) into frontend/public/vosk/ so vite copies it
# into dist and it ships inside the app — no download step for users. Safe to
# skip offline: the mic button then reports the model as missing.
fetch-vosk-model:
	scripts/fetch-vosk-model.sh

# build/ is gitignored and wiped by `make build`, and wails regenerates a
# default Info.plist when it's missing — that default lacks
# NSMicrophoneUsageDescription, and macOS kills the app the moment it touches
# the mic without one. Keep the real plists tracked in packaging/darwin/.
darwin-plist:
ifeq ($(UNAME_S),Darwin)
	mkdir -p build/darwin
	cp packaging/darwin/Info.plist packaging/darwin/Info.dev.plist build/darwin/
endif

dev: fetch-vosk-model darwin-plist
	$(WAILS) dev

sync-config:
	jq --arg v "$(VERSION)" --arg n "$(APP_NAME)" --arg d "$(APP_DESC)" \
		'.info.productVersion=$$v | .info.productName=$$n | .info.comments=$$d | .name=$$n' \
		wails.json > wails.json.tmp && mv wails.json.tmp wails.json
	jq --arg v "$(VERSION)" '.version=$$v' \
		frontend/package.json > frontend/package.json.tmp && mv frontend/package.json.tmp frontend/package.json

build: sync-config fetch-cloudflared fetch-vosk-model
	rm -rf build
	mkdir build
	$(MAKE) darwin-plist
ifeq ($(UNAME_S),Darwin)
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
else
	cp icons/bish_icon.png build/appicon.png
endif
	$(WAILS) build -tags "$(TAGS)" -ldflags "-X main.version=$(VERSION) -X main.appName=$(APP_NAME) -X main.cliName=$(CLI_NAME) -X 'main.cliDescription=$(CLI_DESC)'"

darwin: sync-config
	scripts/fetch-cloudflared.sh darwin
	scripts/fetch-vosk-model.sh
	rm -rf build
	mkdir build
	$(MAKE) darwin-plist
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
	$(WAILS) build -platform darwin/universal -tags "$(TAGS)" -ldflags "-X main.version=$(VERSION) -X main.appName=$(APP_NAME) -X main.cliName=$(CLI_NAME) -X 'main.cliDescription=$(CLI_DESC)'"

install: build
ifeq ($(UNAME_S),Darwin)
	rm -rf /Applications/bish.app
	cp -r build/bin/bish.app /Applications/bish.app
	xattr -dr com.apple.quarantine /Applications/bish.app
else
	mkdir -p $(HOME)/.local/bin $(HOME)/.local/share/applications $(HOME)/.local/share/icons
	cp build/bin/bish $(HOME)/.local/bin/bish
	cp icons/bish_icon.png $(HOME)/.local/share/icons/bish.png
	sed -e 's#@EXEC@#$(HOME)/.local/bin/bish#' -e 's#@ICON@#$(HOME)/.local/share/icons/bish.png#' \
	    -e 's#@NAME@#$(APP_NAME)#' -e 's#@DESC@#$(APP_DESC)#' \
	    packaging/bish.desktop.in > $(HOME)/.local/share/applications/bish.desktop
	-update-desktop-database $(HOME)/.local/share/applications >/dev/null 2>&1
endif
