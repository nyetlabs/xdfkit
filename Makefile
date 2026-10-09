# Build, lint and test xdfkit.
#
#   make                  lint, test, then build
#   make lint             gofmt (must print nothing), go vet, staticcheck
#   make test             go test ./...
#   make build            build/xdfkit and its alias symlinks (xdf2kp, kp2xdf, kp2json)
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
LDFLAGS := -ldflags "-X go.nyet.org/xdfkit/api.Version=$(VERSION)"
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

.PHONY: all lint test build check-links install uninstall schema version package corpus corpus-bump clean help

# Archive name uses macos; the Go port is darwin.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

# xdfkit converts when invoked by one of these names (cmd/xdfkit/alias.go).
ALIASES := xdf2kp kp2xdf kp2json

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

# build replaces build/xdfkit only when it changed, so publish/ rebuilds
# outputs only after a real change.
build:
	mkdir -p build
	go build $(LDFLAGS) -o build/xdfkit.new ./cmd/xdfkit
	if cmp -s build/xdfkit.new build/xdfkit; then rm build/xdfkit.new; else mv build/xdfkit.new build/xdfkit; fi
	for a in $(ALIASES); do ln -sfn xdfkit build/$$a; done

check-links: build
	@set -e; d=$$(mktemp -d); trap 'rm -rf "$$d"' EXIT; \
	cp testdata/archive/ecuxplot/test8maps.kp "$$d/p.kp"; \
	build/kp2xdf -m "$$d/p.kp"; build/kp2json "$$d/p.kp"; rm "$$d/p.kp"; \
	build/xdf2kp "$$d/p.xdf"

# install doesn't build, so sudo make install doesn't run go as root.
install:
	@case $$(uname -s) in Darwin|Linux) ;; *) echo "make install supports macOS and Linux only"; exit 1;; esac
	@test -x build/xdfkit || { echo "build/xdfkit missing: run make build first"; exit 1; }
	install -d "$(DESTDIR)$(LIBDIR)" "$(DESTDIR)$(BINDIR)"
	install -m 0755 build/xdfkit "$(DESTDIR)$(LIBDIR)/xdfkit"
	for a in $(ALIASES); do ln -sfn xdfkit "$(DESTDIR)$(LIBDIR)/$$a"; done
	for n in xdfkit $(ALIASES); do ln -sfn "$(LIBDIR)/$$n" "$(DESTDIR)$(BINDIR)/$$n"; done

# uninstall removes only BINDIR links that point into LIBDIR.
uninstall:
	for n in xdfkit $(ALIASES); do \
		f="$(DESTDIR)$(BINDIR)/$$n"; \
		if [[ -L $$f && $$(readlink "$$f") == "$(LIBDIR)/$$n" ]]; then rm "$$f"; fi; \
		rm -f "$(DESTDIR)$(LIBDIR)/$$n"; \
	done
	rmdir "$(DESTDIR)$(LIBDIR)" 2>/dev/null || true

version:
	@echo $(VERSION)

# Each archive is xdfkit, its aliases, README.md, COPYING and COPYING.LESSER. macOS and Linux are tar.gz,
# with the aliases as symlinks. Windows is zip, with the aliases as .cmd shims.
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
		for a in $(ALIASES); do \
			if [[ $$goos == windows ]]; then printf '@"%%~dp0xdfkit.exe" %s %%*\r\n' "$$a" > "$$stage/$$a.cmd"; else ln -s xdfkit "$$stage/$$a"; fi; \
		done; \
		cp README.md COPYING COPYING.LESSER "$$stage/"; \
		if [[ $$goos == windows ]]; then \
			( cd dist && COPYFILE_DISABLE=1 zip -r -q -X "$$name.zip" "$$name" ); \
		else \
			COPYFILE_DISABLE=1 tar -C dist -czf "dist/$$name.tar.gz" "$$name"; \
		fi; \
		rm -rf "$$stage"; \
	done

# --checkout overrides update = none in .gitmodules.
corpus:
	git submodule update --init --checkout --depth 1 corpus

corpus-bump:
	git submodule update --init --checkout --remote --depth 1 corpus
	@git -C corpus log -1 --format='corpus now at %h %s'
	@git status --short corpus

clean:
	rm -rf build dist

help:
	@sed -n '2,17p' Makefile
