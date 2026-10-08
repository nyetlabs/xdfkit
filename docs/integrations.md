# Integrations

How xdfkit relates to the neighbouring projects. None of them is vendored into this repo. All of them except untyped live in the `nyetlabs` GitHub organization (ecuxplot, ME7Sum and me7-logger moved there from `nyetwurk` on 2026-10-07; GitHub redirects the old URLs).

- [ecu-corpus](corpus.md): the shared test corpus of OEM images and canonical JSON, in the private repo `github.com/nyetlabs/ecu-corpus`, a git submodule (`corpus/`, shallow, `update = none`) of xdfkit, and to be added to ecuxplot, ME7Sum and me7-logger. Its tooling (`tools/`, including `corpus-manifest`) is a separate Go module inside it. Without access, corpus tests skip; CI reads it through a GitHub App token and sets `XDFKIT_REQUIRE_CORPUS=1` so they fail instead (corpus.md, Access and CI credential).
- [me7-logger](https://github.com/nyetlabs/me7-logger) (MIT): a Go module dependency (`go.nyet.org/me7-logger`), pinned to a tag, used behind one package (`me7/`) so the rest of xdfkit doesn't depend on its types. Not wired in yet. What xdfkit uses from it, all ME7 only and all needing the flash image:
  - RAM measurement variables (A2L MEASUREMENT), stored in the model's `measurements` and therefore in the canonical JSON and the XDF metadata file.
  - Located maps, to cross-check a definition's addresses and offer missing maps and axis data.
  - Identification (part number, software version) for project metadata and for warning when a definition is used with a different image.
- [ecuxplot](https://github.com/nyetlabs/ecuxplot): the reference KP reader (mapdump, `org.nyet.mappack`) that `kp/` is ported from. Its original KP packs and their mapdump CSVs are archived in xdfkit's `testdata/archive/ecuxplot/` (copied from `data/` at `a72b8d3`), which the tests read; ecuxplot isn't needed to run them. Its `data/` moved to ecu-corpus as model JSON on 2026-10-08 and is deleted from ecuxplot; xdfkit's `publish/` generates and uploads the KP, CSV and XDF outputs from that JSON.
- [ME7Sum](https://github.com/nyetlabs/ME7Sum): checksum checker; supplies images to the corpus and the OEM checks behind ecu-corpus's `corpus-manifest -me7sum`. No code dependency.
- Both xdfkit and me7-logger write XDF: me7-logger from its located maps, xdfkit from the model (with stamp and metadata file).

## Go import paths

Decided 2026-10-07: Go modules use vanity paths on `go.nyet.org`, so a repo can move between GitHub owners (or hosts) without changing any import. `go.nyet.org` (live since 2026-10-07) answers `?go-get=1` requests with a `go-import` meta tag pointing at the current repo, and redirects browsers to the repo.

| Module | Repo |
| --- | --- |
| `go.nyet.org/xdfkit` | `github.com/nyetlabs/xdfkit` |
| `go.nyet.org/me7-logger` | `github.com/nyetlabs/me7-logger` |
| `go.nyet.org/ecu-corpus/tools` (module in the `tools/` directory, tags `tools/vX.Y.Z`) | `github.com/nyetlabs/ecu-corpus` (private) |
| `go.nyet.org/untyped` | `gitlab.com/nyetwurk/untyped` |

- The corpus tools module lives in a subdirectory so that fetching it never downloads the images. Because the repo is private, users set `GOPRIVATE=go.nyet.org/ecu-corpus` and need read access.
- `xdfkit` is still a placeholder name; renaming the project changes its module path once more.
- me7-logger's tags up to v0.0.2 still declare `module me7-logger`; xdfkit can depend on it once a tag with the `go.nyet.org` path exists.
- untyped's GitLab repo is private, so fetching `go.nyet.org/untyped` needs `GOPRIVATE` and GitLab access. xdfkit doesn't depend on it.

## Git clone URLs

The same host, also as `git.nyet.org`, redirects git over HTTPS for every repo in the table and for ecuxplot and ME7Sum: `git clone https://git.nyet.org/ecuxplot` (or `.../ecuxplot.git`) clones from wherever the repo currently lives. Fetch and push work the same way (push authenticates against GitHub). SSH URLs can't be redirected and stay host-specific.
