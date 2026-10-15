# Build, lint and test xdfkit.
#
#   make                  lint, test, then build
#   make lint             gofmt (must print nothing), go vet, staticcheck
#   make test             go test ./...
#   make build            build/xdfkit and its alias symlinks (ALIASES)
#   make check-links      run the alias symlinks in build/ on a test pack
#   make install          install build/xdfkit (macOS, Linux; run make build first) to LIBDIR
#                         (/usr/local/lib/xdfkit) with its aliases, linked from BINDIR (/usr/local/bin)
#   make uninstall        remove what make install put there
#   make schema           regenerate model/schema.json and model/kp-defaults.json
#   make version          git describe (tags vX.Y.Z and vX.Y.Z-rcN)
#   make package          dist archives for macos, linux, and windows
#   make corpus           fetch the corpus submodule at its pinned commit (needs access)
#   make corpus-bump      move the corpus submodule to the ecu-corpus head (commit it yourself)
#
# Version comes from git tags only. Do not edit a version by hand.

SHELL := /bin/bash

VERSION ?= $(patsubst v%,%,$(shell git describe --tags --match 'v[0-9]*' --dirty --always 2>/dev/null))
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

.PHONY: all lint test build check-links install uninstall schema version package corpus bump corpus-bump clean help

# Archive name uses macos; the Go port is darwin.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

# xdfkit converts when invoked by one of these names (cmd/xdfkit/alias.go).
ALIASES := xdf2kp kp2xdf kp2json xdf2json json2kp

# make install: LIBDIR holds the binary and its alias links, like a release
# archive; BINDIR gets links to them. LIBDIR=/usr/local/xdfkit also works.
# DESTDIR stages the tree for a package.
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
LIBDIR ?= $(PREFIX)/lib/xdfkit

all: lint test build

lint:
	@out=$$(gofmt -l .); if [[ -n $$out ]]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
	go run $(STATICCHECK) ./...

test:
	go test ./...

schema:
	go test ./model -run "TestSchemaFile|TestKPDefaultsFile" -update

# build replaces build/xdfkit only when it changed, so a caller that
# checks the binary's mtime rebuilds only after a real change.
build:
	scripts/build $(VERSION) $(ALIASES)

check-links: build
	scripts/check-links

# install doesn't build, so sudo make install doesn't run go as root.
install:
	DESTDIR='$(DESTDIR)' LIBDIR='$(LIBDIR)' BINDIR='$(BINDIR)' scripts/install $(ALIASES)

# uninstall removes only BINDIR links that point into LIBDIR.
uninstall:
	DESTDIR='$(DESTDIR)' LIBDIR='$(LIBDIR)' BINDIR='$(BINDIR)' scripts/uninstall $(ALIASES)

version:
	@echo $(VERSION)

# Each archive is xdfkit, its aliases, README.md, COPYING and COPYING.LESSER.
package:
	scripts/package '$(VERSION)' '$(ALIASES)' '$(PLATFORMS)'

# --checkout overrides update = none in .gitmodules.
corpus ecu-corpus/corpus.tsv:
	git submodule update --init --checkout --depth 1 ecu-corpus

bump corpus-bump:
	git submodule update --init --checkout --remote --depth 1 ecu-corpus
	@git -C ecu-corpus log -1 --format='corpus now at %h %s'
	@git status --short ecu-corpus

clean:
	rm -rf build dist

help:
	@sed -n '2,17p' Makefile
