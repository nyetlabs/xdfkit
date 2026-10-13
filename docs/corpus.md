# Test corpus

`ecu-corpus` (`github.com/nyetlabs/ecu-corpus`, private) supplies the OEM flash images and `categories.json` that xdfkit's tests read. It is the `corpus/` submodule (shallow, `update = none`). Tests do not read `defs/`.

## What tests read

- `images/<part>/NAME.bin`, `<part>` the first 9 characters of the part number. Named from `testdata/archive/ecuxplot/images.tsv` (`internal/corpus.ArchiveImage`). `8N0906018CB` uses the bin in that archive: the corpus image of that part is different software.
- `categories.json`, checked by `model.TestCorpusCategoryTable` against `model/categories.schema.json`.
- `corpus.tsv`. `internal/corpus.TestImages` checks every row's size and SHA-256 against `images/<part>/`. Lookups are by name or SHA-256.

Without a manifest those tests skip. `XDFKIT_REQUIRE_CORPUS=1` makes that a failure. CI sets it when it has the credential. Lint also has synthetic fixtures that need no corpus (`testdata/lint/cases.json`).

## Where the checkout is

`XDFKIT_CORPUS` if set, else `../ecu-corpus` when that directory has `corpus.tsv`, else `corpus/` at the module root.

```sh
make corpus        # submodule at its pinned commit
make corpus-bump   # submodule at the ecu-corpus head
```

## CI

`.github/workflows/build.yml` checks out `corpus/` with a short-lived token from the `nyetlabs-corpus-reader` GitHub App (organization variable `CORPUS_APP_CLIENT_ID`, secret `CORPUS_APP_KEY`), for the `ecu-corpus` URL only. Fork PRs have no secrets and skip. `.github/workflows/corpus-bump.yml` moves the submodule to the corpus head and opens a pull request. `release.yml` reuses the build and must pass `secrets: inherit`.
