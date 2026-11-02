# Integrations

How xdfkit relates to neighbouring projects. None is vendored here. All except untyped live in the `nyetlabs` GitHub organization (old `nyetwurk` URLs redirect).

- [ecu-corpus](corpus.md): private OEM flash images and category table for tests.
- [me7-logger](https://github.com/nyetlabs/me7-logger) (MIT): imports `model`, `xdf`, `canon` (`go.nyet.org/xdfkit`); xdfkit never imports it. me7info builds model JSON (provenance format `image`, origin `auto`), uses `Model.Tuner` / `Categorize`, and writes XDF through xdfkit. Pins xdfkit (pseudo-version of `master` between releases; tag for releases) and develops against a local checkout via gitignored `go.work`; run `make check-pinned` there before pushing.
- [ecuxplot](https://github.com/nyetlabs/ecuxplot): reference KP reader (`org.nyet.mappack`) that `kp/` is ported from. Archived packs and mapdump CSVs live in `testdata/archive/ecuxplot/`; ecuxplot is not needed to run tests.
- [ME7Sum](https://github.com/nyetlabs/ME7Sum): checksum checker. No code dependency.
