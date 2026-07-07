WAILS := $(shell go env GOPATH)/bin/wails

.PHONY: init dev build install darwin

init:
	go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
	go mod download
	cd frontend && pnpm install

dev:
	$(WAILS) dev

build:
	rm -rf build
	mkdir build
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
	$(WAILS) build

darwin:
	rm -rf build
	mkdir build
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
	$(WAILS) build -platform darwin/universal

install: build
	rm -rf /Applications/bish.app
	cp -r build/bin/bish.app /Applications/bish.app
	xattr -dr com.apple.quarantine /Applications/bish.app
