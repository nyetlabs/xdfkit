# Build, lint and test xdfkit.
#
#   make                  lint, test, then build
#   make lint             gofmt (must print nothing), go vet, staticcheck
#   make test             go test ./...
#   make build            build/xdfkit
#   make version          git describe (tags vX.Y.Z and vX.Y.Z-rcN)
#   make package          dist archives for macos, linux, and windows
#
# Version comes from git tags only. Do not edit a version by hand.

SHELL := /bin/bash

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --match 'v[0-9]*' --dirty --always 2>/dev/null))
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

.PHONY: all lint test build version package clean help

# Archive name uses macos; the Go port is darwin.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

all: lint test build

lint:
	@out=$$(gofmt -l .); if [[ -n $$out ]]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	go run $(STATICCHECK) ./...

test:
	go test ./...

build:
	mkdir -p build
	go build $(LDFLAGS) -o build/xdfkit ./cmd/xdfkit

version:
	@echo $(VERSION)

# Each archive is xdfkit, README.md, COPYING and COPYING.LESSER. macOS and Linux are tar.gz. Windows is zip.
package:
	rm -rf dist
	mkdir -p dist
	@set -euo pipefail; \
	command -v zip >/dev/null; \
	for spec in $(PLATFORMS); do \
		goos=$${spec%/*}; \
		arch=$${spec#*/}; \
		os=$$goos; \
		ext=; \
		if [[ $$goos == darwin ]]; then os=macos; fi; \
		if [[ $$goos == windows ]]; then ext=.exe; fi; \
		name=xdfkit-$(VERSION)-$$os-$$arch; \
		stage=dist/$$name; \
		rm -rf "$$stage"; \
		mkdir -p "$$stage"; \
		CGO_ENABLED=0 GOOS=$$goos GOARCH=$$arch go build $(LDFLAGS) -o "$$stage/xdfkit$$ext" ./cmd/xdfkit; \
		cp README.md COPYING COPYING.LESSER "$$stage/"; \
		if [[ $$goos == windows ]]; then \
			( cd dist && COPYFILE_DISABLE=1 zip -r -q -X "$$name.zip" "$$name" ); \
		else \
			COPYFILE_DISABLE=1 tar -C dist -czf "dist/$$name.tar.gz" "$$name"; \
		fi; \
		rm -rf "$$stage"; \
	done

clean:
	rm -rf build dist

help:
	@sed -n '2,10p' Makefile
