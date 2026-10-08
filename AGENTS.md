# Agent handoff

Start here. This repo is the proof of concept for a command-line converter between map definition formats (WinOLS KP, A2L, DAMOS, TunerPro XDF, WinOLS scripts, and a lossless JSON/YAML model), plus an autocorrect for KP files. The user is the author of some of the KP files and wants to fix them.

## Name

`xdfkit` is a placeholder for the PoC only (Go module `go.nyet.org/xdfkit`, repo `github.com/nyetlabs/xdfkit`). The real name is undecided. Earlier drafts said `mapxlate`. When a name is chosen, rename it everywhere, including the XDF embedded-model marker (`docs/xdf-embedding.md`).

## Status (2026-10-07)

- Language: Go, pure (no cgo) in the core packages and the CLI; only the planned C ABI library (`capi/`) may use cgo. Rationale and the bindings design are in `docs/design.md`, Decisions. Keep core packages on bytes, without file or console I/O. Licence: LGPL-3.0-or-later (`COPYING.LESSER`, `COPYING`).
- `kp/`: KP v1/v2 reader and writer, ported from ecuxplot's `org.nyet.mappack`. It matches mapdump's CSV output for all 15 packs in ecuxplot's `data/` (32,307 maps).
 - The record layouts are the Go structs (`File`, `Project`, `Folder`, `Map`, `Axis`), coded in declaration order by one reflection walker in `codec.go`, for both decoding and encoding. `kp:"..."` tags cover layout-only fields, raw lengths and length-prefixed lists; hooks cover the header and the v2 zip. Undecoded bytes are kept as `Hex` raw blocks.
 - Undecoded fields are named `Unk` plus mapdump's `hN` name (`UnkH9a`, JSON `unkH9a`): placeholders that should all get real names eventually. Guessed names say so in a comment (`Range`, `Addr2`, `Signature`).
 - `File.Offsets(paths...)` finds the offsets of any fields by JSON path in one walk (for the patch-mode writer); `File.Offset(path)` is the single-path form.
 - `File.Encode` round-trips all 16 packs, which passes the stage 1 feasibility gate (`kp-writer-rt`): v1 byte-identical, v2 identical except for the re-deflated zip. See `docs/kp-format.md`, Round trip. A WinOLS load test is still pending.
 - The JSON is `kp.File`'s own encoding, not the canonical model.
- `canon/`: `jq -S .` printer and JSON edit stamp (`docs/stamp-and-metadata.md`). Type-guided JSON coding (`typed.go`): numbers are spelled by Go type (whole floats as `1.0`), and `canon.Unmarshal` accepts any exact whole literal for integer fields and names the JSON path in errors. The stamp carries two digests, RFC 8785 (via `github.com/gowebpki/jcs`, the only dependency) and `jq -S .`, kept side by side until we learn which to keep; `verify` reports `mixed` when they disagree. Numbers below 1e-6 use jq 1.8's exponent spelling, since real factors need it. Tests compare against real jq (skip if jq is missing).
- `cmd/xdfkit`: `xdfkit [-f json|kp] [-force] input [output]` converts KP to stamped canonical JSON and back (JSON read through `canon.Unmarshal`, input detected from contents, `-` for stdin, never overwrites without `-force`); `xdfkit verify file.json...` prints clean, edited, mixed, unknown or unstamped, with each digest's result. Loading edited JSON warns.
- Corpus: `github.com/nyetlabs/ecu-corpus` (private) holds the 99 OEM images in `images/`, root symlinks, `corpus.tsv` and its tooling (`corpus-manifest`, `corpus-links`, `grant-ci-access`, CI templates), moved out of xdfkit on 2026-10-07. The GitHub App and the organization-level CI credential exist. It is xdfkit's `corpus/` submodule (shallow, `update = none`; fetch with `git submodule update --init --checkout corpus`), fetched in CI with the app token; tests use `internal/corpus` (skip without it, fail with `XDFKIT_REQUIRE_CORPUS=1`). Not yet in the other consumers.
- Hosting: all repos except untyped are in the `nyetlabs` organization. `go.nyet.org` (alias `git.nyet.org`) is live: Go vanity imports and HTTPS git clone redirects (`docs/integrations.md`).
- Not started: canonical model and JSON Schema, YAML, lint/fix, XDF writer and sidecar, WinOLS script writer, A2L and DAMOS readers, me7-logger integration.
- The user committed and pushed the initial xdfkit (`github.com/nyetlabs/xdfkit`) and ecu-corpus on 2026-10-07. Do not commit unless the user asks.

## Build and test

```sh
make          # lint (gofmt, go vet, staticcheck), test, build
make test
```

- Tests read ecuxplot's `data/*.kp` and their `.csv` (mapdump output) as the oracle, from `XDFKIT_ECUXPLOT_DATA` or a sibling checkout at `../ecuxplot/data`, and skip when neither exists (`docs/integrations.md`).
- mapdump's CSV doesn't escape quotes in names, so maps whose names contain `"` are skipped in that comparison.
- Run `make lint` before handing back; it must pass. There is no toolchain directive in `go.mod`; keep it that way.
- `XDFKIT_REQUIRE_DATA=1` turns missing ecuxplot data or jq into test failures (CI sets it, and installs jq 1.8 and checks out ecuxplot's `data/`).
- CI: `.github/workflows/build.yml` (push to master, PRs) runs `make lint`, `make test`, `make build`; `release.yml` publishes on `vX.Y.Z` tags with git-cliff notes. Same shape as me7-logger.
- Commit messages start with a prefix that `cliff.toml` groups: `feat`/`add`, `fix`, `docs`, `refactor`, `chore`, `ci`, `build`, `test`, `style`, `chore(deps)`; anything else lands under Other.

## Docs

`docs/` is tracked and describes how things are, including spec that isn't implemented yet.

- `docs/design.md`: decisions, architecture diagram, repo layout and CLI, verification criteria.
- `docs/format-matrix.md`: which format supports which field (39 rows, with totals).
- `docs/kp-format.md`: KP v1/v2 layout as understood so far, enums, v2 zip details, "EEPROM, subtract" axes.
- `docs/autocorrect.md`: `lint` and `fix` design, rules R1 to R6, patch-mode writer.
- `docs/winols-script.md`: WinOLS script import format (the primary non-KP route into WinOLS), with sample and property table.
- `docs/stamp-and-metadata.md`: the hand-edit stamp (SHA-256 digests over RFC 8785 and `jq -S .` text) for JSON and XDF, typed number handling, and the XDF metadata sidecar that makes XDF output lossless.
- `docs/xdf-embedding.md`: optional embedding of the sidecar payload in an XDF comment.
- `docs/corpus.md`: shared corpus repo `ecu-corpus` for all four projects, holding images plus canonical JSON (other formats generated by xdfkit); repo `github.com/nyetlabs/ecu-corpus`, checked out locally in `../ecu-corpus`.
- `docs/naming.md`: file naming convention for corpus images (`PART-VERSION[-label].bin`, from the image's own identification), definitions (`STEM.OWNER.json`) and xdfkit outputs.
- `docs/integrations.md`: how ecu-corpus (submodule), me7-logger (Go module), ecuxplot (test data) and ME7Sum relate to xdfkit; Go import paths and git clone URLs on `go.nyet.org`.

## Constraints

- Git: never commit, push, tag, checkout, stash, reset or rebase unless the user asks. GitHub is read-only.
- Do not decompile, unpack or bypass WinOLS's protection, its loader, or its key files. `ols.exe` and the plugins are packed.
- Assume users have no licensed WinOLS options (no Damos plugin).
- XDF Porter may be used as an oracle only with already-public files (ecuxplot `data/*.kp`). Never upload the user's private files. DAMOS, A2L and OLS files are private and of questionable provenance: they live only in xdfkit's gitignored `testdata/local/`. Never commit them, never un-ignore that directory, and never put them or other original definition files in the shared corpus, which holds only OEM images and JSON (hard decisions, `docs/corpus.md`).
- Don't modify ecuxplot's mapdump until the user asks.
- GUI checks (WinOLS, TunerPro) are done by the user. Files for them go in `testdata/local/outgoing/`; files the user brings back go in `testdata/local/incoming/`.
- Follow `.cursor/rules/` and the user's global Cursor rules (Go: gofmt; Markdown: blank lines around lists and fences, unnumbered lists).

## Plans

Local setup (paths, tools, sibling checkouts), work in progress, what is waiting on the user, next steps, the roadmap and todo checklist live in `plans/` (local, gitignored, not authoritative), starting at `plans/README.md`. See `.cursor/rules/docs-plans.mdc`.
