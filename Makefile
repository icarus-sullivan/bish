WAILS := $(shell go env GOPATH)/bin/wails
UNAME_S := $(shell uname -s)
TAGS   ?=

VERSION  := $(shell scripts/read-config.sh version)
APP_NAME := $(shell scripts/read-config.sh app.name)
APP_DESC := $(shell scripts/read-config.sh app.description)
CLI_NAME := $(shell scripts/read-config.sh cli.name)
CLI_DESC := $(shell scripts/read-config.sh cli.description)

.PHONY: init dev build install darwin sync-config

init:
	go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
	go mod download
	cd frontend && pnpm install

dev:
	$(WAILS) dev

sync-config:
	jq --arg v "$(VERSION)" --arg n "$(APP_NAME)" --arg d "$(APP_DESC)" \
		'.info.productVersion=$$v | .info.productName=$$n | .info.comments=$$d | .name=$$n' \
		wails.json > wails.json.tmp && mv wails.json.tmp wails.json
	jq --arg v "$(VERSION)" '.version=$$v' \
		frontend/package.json > frontend/package.json.tmp && mv frontend/package.json.tmp frontend/package.json

build: sync-config
	rm -rf build
	mkdir build
ifeq ($(UNAME_S),Darwin)
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
else
	cp icons/bish_icon.png build/appicon.png
endif
	$(WAILS) build -tags "$(TAGS)" -ldflags "-X main.version=$(VERSION) -X main.appName=$(APP_NAME) -X main.cliName=$(CLI_NAME) -X 'main.cliDescription=$(CLI_DESC)'"

darwin: sync-config
	rm -rf build
	mkdir build
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
