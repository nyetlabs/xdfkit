# Shared test corpus

`ecu-corpus` (`github.com/nyetlabs/ecu-corpus`, private): OEM flash images and their map definitions as canonical JSON, shared by xdfkit, ME7Sum and me7-logger as a git submodule (`corpus/`). Its tools (`tools/`, Go module `go.nyet.org/ecu-corpus/tools`): `corpus-manifest` builds, checks and classifies; `corpus-links` makes the root symlinks.

Contents: 99 OEM images seeded from the source repos, the manifest, CI check, the ecuxplot definitions as JSON and `categories.json`. Consumers fetch it in CI and run the weekly bump workflow (`.github/workflows/corpus-bump.yml`; each repo must allow Actions to create pull requests). Test access: xdfkit `internal/corpus`, me7-logger `internal/ecucorpus`, ME7Sum `scripts/test.sh`; none commits corpus images any more (ME7Sum keeps its non-OEM ones in `testdata/`). ecuxplot has no images and no submodule.

## Specification

### Rules

- Only images (`.bin`), canonical JSON and the manifest. Never KP, XDF, DAMOS, A2L, OLS, hex or any other original definition file (DAMOS and A2L provenance is often questionable); JSON converted from them does go in, with their names, descriptions and conversions. Private files are never uploaded to third-party services (XDF Porter or similar).
- Contributed images are welcome (the corpus should go public eventually) under the OEM rules below, with `release` = `unknown`.
- OEM (factory, unmodified) images only; modified, tuned, re-signed or identification-wiped ones stay in their source repos or xdfkit `testdata/`. `tools/corpus-manifest -me7sum PATH -overrides corpus-overrides.tsv` runs ME7Sum and writes the `oem`, `release`, `checksums` and `rsa` columns. `rsa`, `checksums` and `hand` qualify. `release` stays `unknown` (all 99 at seeding) until set by hand, and must be `production` before the corpus could go public; a non-production image found later is removed, but stays in the private history, so going public needs a fresh history.

| `oem` | Meaning |
| --- | --- |
| `rsa` | ME7Sum finds an RSA signature, the modulus is Bosch's, and the signed MD5 matches the computed one. Strong evidence. |
| `checksums` | No RSA signature (most ME7.1 and some ME7.5); identification intact and every ME7Sum checksum and CRC passes. Weaker: a tuned image with corrected checksums also passes, and so does a genuine Bosch calibration that never went into production. |
| `no` | Labelled (modified), foreign modulus (re-signed), Bosch modulus with a failing MD5 (signed regions changed), checksum errors, or left out by hand. |
| `hand` | ME7Sum can't check the family; judged OEM by the user (`corpus-overrides.tsv`). |
| `review` | Family ME7Sum doesn't check, ME7Sum fails outright, or the name needs a hand decision; resolved by adding a row to `corpus-overrides.tsv`. |

- Bosch modulus: SHA-256 `3565ec2768952ec00ce0a4ffaa9ce851408e29bb2adf4731017aa31cfb735d27` of the 128-byte modulus (exponent 3). Any other modulus means the image was re-signed.
- Strengthening `checksums`: two `checksums` images with the same part and version but different bytes would mean one is modified. A second source of the same image (another dump of the same part and version) also counts.
- Part number and version come from ME7Sum's reading of the ECUID table when `-me7sum` is given (fixed-width fields), and from a text match otherwise.
- `release` (`production`, `non-production`, `unknown`) is set by hand: no check tells a production calibration from a genuine unreleased one (development, pre-series).

### What goes in: images and canonical JSON

The schema is meant to represent everything any of the projects needs. Other formats are generated from the JSON by xdfkit, and other projects pull in xdfkit rather than keeping format code of their own.

- One JSON per definition, converted by xdfkit from the original (KP, hand-made XDF, DAMOS, A2L). An image can have several definitions (a KP, a dated KP, me7-logger's hand XDF, a DAMOS file).
- Each JSON records its provenance: source format, original file name and SHA-256, and xdfkit version. So the original stays identifiable even though it isn't in the corpus.
- And the definitions' origin (`provenance.origin`): `damos` or `a2l` (from Bosch data, the reference for map addresses) or `hand` (as WinOLS map packs usually are); me7-logger's parity check relies on it. The DAMOS and A2L readers will set it; KP and XDF input take `xdfkit -origin` (default `hand`, or an XDF metadata file's). The publish import keeps the corpus JSON's origin unless given `ORIGIN=`, but makes a `hand` definition with 3000 or more maps `damos` (hand packs have at most about 540, most DAMOS exports over 4000); smaller exports (8D0907558E about 1800, 8D0907558M about 400) need `ORIGIN=damos`.
- The edit stamp (`stamp-and-metadata.md`): clean means converted, regenerate at will; edited means a person changed it and it is the source of truth, never overwritten by regeneration.
- One category table, `categories.json` (schema `model/categories.schema.json`, `model.CategoryTable`): `{"schema": 1, "categories": {"KFZW": "Timing", ...}}`. Names: the tuner list (me7-logger `testdata/parity/names/tuner.yaml`, from the S4wiki tuning page); categories drafted from 8D0907551M's folders, then edited by hand. The 1.8T block list (about 4,100 more names) is left out: it covers most of any ME7 DAMOS (tuner XDFs kept 75-97% of maps). `xdfkit -tuner` and `publish/` (ME7 packs only) write subsets from it, marked `provenance.subset` (`model.md`). me7-logger ships a copy as `config/categories.json`, so a rename takes a corpus commit plus a bump in each consumer.
- Per-image tool data that isn't a map definition (ME7Sum's `.ini` checksum layout) goes into the schema as its own section if a consumer needs it from the corpus. Otherwise it stays with the tool.
- No tool outputs: goldens (mapdump CSV and XDF, ME7Sum `.bin.txt`, me7info XDF and `.ecu`) stay with the tool that produces them, keyed by corpus id (mapdump's CSVs for ecuxplot's packs are archived in xdfkit).

### Where the original files live

Readers need originals for regression tests (DAMOS and A2L have no writers; exact KP bytes need the file).

- Private originals (DAMOS, A2L, OLS and their hex files) live only in a checkout's gitignored `testdata/local/`, never committed or pushed anywhere. Tests that need them skip when they are absent.
- Public originals are read from the repos that publish them or copied into xdfkit `testdata/`. ecuxplot's packs and mapdump CSVs are in `testdata/archive/ecuxplot/`: never edited, the KP reader's oracle; their definitions are maintained as corpus JSON.
- Each original is checked against the provenance hash in its corpus JSON, where one exists.

### Layout and ids

- `images/PART-VERSION[-label].bin`: the images, and `defs/PART-VERSION.json`: the definitions (first, ecuxplot's packs), named as specified in `naming.md`. Part number and software version come from the image itself, so names are deterministic. Tests and tools use these paths only.
- Root symlinks into `images/` for browsing (`tools/corpus-links`, checked by `corpus-manifest -check`): `PART.bin` while a part has one image, else `PART-VERSION[-label].bin`. Names change when a part gains an image, so tools don't use them.
- `corpus.tsv` at the root, generated by `tools/corpus-manifest -corpus images` and committed. Columns: name, sha256, size, family, ident, oem, release, checksums, rsa, def (the image's definition, `defs/NAME.json`, or `-`). `corpus-manifest -check` (CI) also fails on a `def` that doesn't match `defs/` and on files in `defs/` not named after an image. Tests look files up by id or hash through the manifest, never by guessing paths.
- `categories.json` at the root, outside `defs/`, so `corpus-manifest -check` ignores it.

### Policy

- Images are immutable: a changed image gets a new id. JSON files do change, through ordinary commits: hand fixes, and bulk regeneration when the schema or a reader improves. Bulk regeneration skips JSON with an edited stamp.
- Schema versions: every JSON names its schema version. xdfkit reads older versions, or the corpus is regenerated in one commit per schema bump. Consumers pin a corpus commit and an xdfkit version that agree.
- Canonical form (`jq -S .`, one value per line) keeps JSON diffs small and reviewable in git.
- Plain git, not LFS (about 100 MB; LFS adds a tool and bandwidth quotas). Consumers use `shallow = true` in `.gitmodules`.
- Edits go in a full clone, `ecu-corpus` beside the consumer repos (`../ecu-corpus`): commit and push there, then `make corpus-bump` in each consumer. The `corpus/` submodules are read-only: shallow, so `git log` may not reach a JSON file's last commit and dates its zip wrongly, and `corpus-bump` detaches them, so work left there is lost. xdfkit's `publish/` and `import-incoming.sh` use `../ecu-corpus` when it exists, else the submodule (`CORPUS=` overrides).

### Access: private repo

Private for now; consumer repos can be public (only the submodule URL and commit show).

- Without access `corpus/` is empty and corpus tests skip, naming this document. Go modules never include submodule contents.
- xdfkit tests (`internal/corpus`) use `XDFKIT_CORPUS`, else `../ecu-corpus` if it has a `corpus.tsv` (so local edits are tested before a bump), else `corpus/`; no `corpus.tsv` means not available. Images are looked up by name or SHA-256.
- `XDFKIT_REQUIRE_CORPUS=1` makes not available a failure; CI sets it whenever it has the credential, so a dead credential fails the build. Fork PRs get no secrets and skip; don't use `pull_request_target` to get around that unless a maintainer gates the run.
- `.gitmodules`: absolute URL `https://github.com/nyetlabs/ecu-corpus.git` (a relative one breaks forks; SSH users map it with `url.git@github.com:.insteadOf`), and `update = none`, so `git submodule update --init` doesn't fail without access; with access, `git submodule update --init --checkout corpus` (or `make corpus`). Unconfirmed: `git clone --recurse-submodules` with this setting.
- Developers: collaborator read access, own GitHub login. CI credentials are automated: no long-lived personal tokens or hand-copied keys (`GITHUB_TOKEN` can't read another private repo).

### CI credential: GitHub App

Templates: ecu-corpus `tools/consumer-ci/`.

- App `nyetlabs-corpus-reader`, owned by the `nyetlabs` organization: Contents read only, no webhooks, installed only on `ecu-corpus`.
- Organization Actions variable `CORPUS_APP_CLIENT_ID` (client ID; `actions/create-github-app-token` v3 deprecates the numeric App ID) and secret `CORPUS_APP_KEY` (private key), visible to all its repos. A consumer outside the organization gets repo-level copies under the same names.
- Each run mints a token (`create-github-app-token` v3, `client-id`, owner `nyetlabs`, repository `ecu-corpus`; about an hour), points git at it for the `ecu-corpus` URL only (`url.<token URL>.insteadOf`), then runs `git submodule update --init --checkout corpus`. A workflow reusing the build through `workflow_call` (release) must pass `secrets: inherit`.
- `tools/grant-ci-access` sets the variable and secret (`gh variable set`, `gh secret set`): no arguments for the organization, repo names for outside consumers.
- Rotation: new key in the app settings (web only, as far as known), re-run `grant-ci-access`, delete the old key. Revocation: delete the key or uninstall the app.
- Bumps: a scheduled workflow per consumer moves the submodule to the corpus head and opens a pull request. Not Dependabot: its secrets are static, so it can't mint the app token.
