# Integrations

How xdfkit relates to the neighbouring projects. None of them is vendored into this repo. All of them except untyped live in the `nyetlabs` GitHub organization (GitHub redirects the old `nyetwurk` URLs).

- [ecu-corpus](corpus.md): the shared test corpus of OEM images and canonical JSON, in the private repo `github.com/nyetlabs/ecu-corpus`, a git submodule (`corpus/`, shallow, `update = none`) of xdfkit, ME7Sum and me7-logger; ecuxplot has no images left and no submodule. Its tooling (`tools/`, including `corpus-manifest`) is a separate Go module inside it. Without access, corpus tests skip; CI reads it through a GitHub App token and sets `XDFKIT_REQUIRE_CORPUS=1` so they fail instead (corpus.md, Access and CI credential).
- [me7-logger](https://github.com/nyetlabs/me7-logger) (MIT): imports xdfkit (`model`, `xdf`, `canon`); xdfkit never imports me7-logger. me7info builds a model from the maps it locates (provenance format `image`, origin `located`), files it with `Model.Tuner` or `Model.Categorize` and writes it with `xdf.Write`; its parity check reads corpus JSON through `canon.Unmarshal` and `Model.Check`. ME7-only data that xdfkit could use (RAM measurements, located maps, identification) is to arrive as model JSON written by me7info, not through a Go dependency. me7-logger pins xdfkit (a pseudo-version of `master` between releases, a tag for its releases) and develops against a local checkout through a gitignored `go.work` (me7-logger `DEVELOPER.md`).
- [ecuxplot](https://github.com/nyetlabs/ecuxplot): the reference KP reader (mapdump, `org.nyet.mappack`) that `kp/` is ported from. Its original KP packs and their mapdump CSVs are archived in xdfkit's `testdata/archive/ecuxplot/` (copied from `data/` at `a72b8d3`), which the tests read; ecuxplot isn't needed to run them. Its definitions are model JSON in ecu-corpus (ecuxplot has no `data/`); xdfkit's `publish/` generates and uploads the KP, CSV and XDF outputs (each XDF with its metadata file) from that JSON.
- [ME7Sum](https://github.com/nyetlabs/ME7Sum): checksum checker; supplies images to the corpus and the OEM checks behind ecu-corpus's `corpus-manifest -me7sum`. No code dependency.
- me7-logger writes XDF through xdfkit.

## Go import paths

Go modules use vanity paths on `go.nyet.org`, so a repo can move between GitHub owners (or hosts) without changing any import. `go.nyet.org` answers `?go-get=1` requests with a `go-import` meta tag pointing at the current repo, and redirects browsers to the repo.

| Module | Repo |
| --- | --- |
| `go.nyet.org/xdfkit` | `github.com/nyetlabs/xdfkit` |
| `go.nyet.org/me7-logger` | `github.com/nyetlabs/me7-logger` |
| `go.nyet.org/ecu-corpus/tools` (module in the `tools/` directory, tags `tools/vX.Y.Z`) | `github.com/nyetlabs/ecu-corpus` (private) |
| `go.nyet.org/untyped` | `gitlab.com/nyetwurk/untyped` |

- The corpus tools module lives in a subdirectory so that fetching it never downloads the images. Because the repo is private, users set `GOPRIVATE=go.nyet.org/ecu-corpus` and need read access.
- me7-logger's tags from v0.0.3 declare `module go.nyet.org/me7-logger`; earlier tags declare `module me7-logger` and can't be imported by that path.
- untyped's GitLab repo is private, so fetching `go.nyet.org/untyped` needs `GOPRIVATE` and GitLab access. xdfkit doesn't depend on it.

## Git clone URLs

The same host, also as `git.nyet.org`, redirects git over HTTPS for every repo in the table and for ecuxplot and ME7Sum: `git clone https://git.nyet.org/ecuxplot` (or `.../ecuxplot.git`) clones from wherever the repo currently lives. Fetch and push work the same way (push authenticates against GitHub). SSH URLs can't be redirected and stay host-specific.
