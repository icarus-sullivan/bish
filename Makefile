WAILS := $(shell go env GOPATH)/bin/wails

dev:
	$(WAILS) dev

build:
	$(WAILS) build
