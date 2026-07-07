WAILS := $(shell go env GOPATH)/bin/wails

dev:
	$(WAILS) dev

build:
	rm -rf build
	mkdir build
	sips -z 1024 1024 icons/bish_icon.png --out build/appicon.png
	$(WAILS) build

install: build
	cp -r build/bin/bish.app /Applications/bish.app
	xattr -dr com.apple.quarantine /Applications/bish.app
