# Shared test corpus

Specified 2026-10-07: `ecu-corpus`, one private GitHub repo, `github.com/nyetlabs/ecu-corpus` (organization `nyetlabs`), of flash images and their map definitions as canonical JSON, added as a git submodule to ecuxplot, ME7Sum, me7-logger and xdfkit. It replaces the copies now spread across those repos. Its tooling (`tools/`, Go module `go.nyet.org/ecu-corpus/tools`) includes `corpus-manifest`, which builds and checks the manifest and classifies images, and `corpus-links`, which makes the root symlinks.

State (2026-10-07): the repo exists and holds the 99 OEM images found in the three source repos, with the manifest, the tools and its own CI check. The GitHub App and the organization-level credential are set up (CI credential, below). xdfkit has the submodule (`corpus/`) and fetches it in CI (`.github/workflows/build.yml`); tests reach it through `internal/corpus`. Not yet done: the bump workflow, and the submodule, CI and removal of their own image copies in ecuxplot, ME7Sum and me7-logger.

## Specification

### Hard decision: images and JSON only

Decided 2026-10-07, not open for revision: the shared corpus contains only images (`.bin`) and canonical JSON, plus its manifest. Never KP, XDF, DAMOS, A2L, OLS, hex or any other original definition file. DAMOS and A2L files often have questionable provenance, and JSON can represent everything the projects need. Images in it are only ones already published in the public ecuxplot, ME7Sum and me7-logger repos.

- Private files are never uploaded to third-party services (XDF Porter or similar), consistent with the existing rule.

### Hard decision: OEM images only

Decided 2026-10-07: the corpus holds only OEM (factory, unmodified) images. Modified, tuned, re-signed or identification-wiped images stay in their source repos or in xdfkit `testdata/` if a test needs them.

How an image is judged (`tools/corpus-manifest -me7sum PATH -overrides corpus-overrides.tsv` in ecu-corpus, which runs ME7Sum and writes the `oem`, `release`, `checksums` and `rsa` manifest columns). Decided 2026-10-07: `rsa`, `checksums` and `hand` images qualify. While the corpus is private, they go in with `release` still `unknown` (all 99 at seeding); `release` must be set to `production` by hand before the corpus could ever be made public, and a non-production image found later is removed in a normal commit (it stays in the private history, so going public would need a fresh history).

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
- Production versus non-production: none of these checks can tell a production calibration from a genuine but unreleased Bosch one (development, pre-series). That needs outside knowledge, so the manifest gets a hand-set `release` column (`production`, `non-production`, `unknown`; default `unknown`), which gates a public corpus (see above).

### What goes in: images and canonical JSON

The corpus holds only images and canonical JSON. It starts with the images and the manifest only (decided 2026-10-07); JSON is added once the JSON Schema exists. The schema is meant to represent everything any of the projects needs. Other formats are generated from the JSON by xdfkit, and other projects pull in xdfkit rather than keeping format code of their own.

- One JSON per definition, converted by xdfkit from the original (KP, hand-made XDF, DAMOS, A2L). An image can have several definitions (a KP, a dated KP, me7-logger's hand XDF, a DAMOS file).
- Each JSON records its provenance: source format, original file name and SHA-256, and xdfkit version. So the original stays identifiable even though it isn't in the corpus.
- The edit stamp (`stamp-and-metadata.md`) separates the two kinds of JSON. A clean stamp means "converted, regenerate at will". An edited stamp means a person changed it (for example an autocorrect fix reviewed by hand), and that JSON is now the source of truth. Regeneration must not overwrite it.
- Per-image tool data that isn't a map definition (ME7Sum's `.ini` checksum layout) goes into the schema as its own section if a consumer needs it from the corpus. Otherwise it stays with the tool.
- Not tool outputs. mapdump's CSV and XDF, ME7Sum's `.bin.txt`, and me7info's generated XDF and `.ecu` are goldens. They stay in the repo of the tool that produces them, keyed by corpus id (exception: mapdump's CSVs for ecuxplot's packs, archived in xdfkit with the packs). Most become unnecessary once those tools read JSON through xdfkit.

### Where the original files live

Originals stay out of the corpus, but xdfkit's readers need them for regression tests. Some can't be regenerated from JSON: DAMOS and A2L have no writers, and exact KP bytes need the original file.

- Private originals (DAMOS, A2L, OLS and their hex files) live only in a checkout's gitignored `testdata/local/`, never committed or pushed anywhere. Tests that need them skip when they are absent.
- Public originals (ecuxplot KP, me7-logger hand XDFs) are read from the repos that publish them, or copied into xdfkit's committed `testdata/` if a test must not depend on another checkout. ecuxplot's KP packs and their mapdump CSVs are archived in `testdata/archive/ecuxplot/` (2026-10-08): an archive, never edited, kept for the exact originals and as the KP reader's test oracle; their definitions are maintained as corpus JSON.
- Each original is checked against the provenance hash in its corpus JSON, where one exists.

### Layout and ids

- `images/PART-VERSION[-label].bin`: the images, and `defs/PART-VERSION.json`: the definitions (first, ecuxplot's packs), named as specified in `naming.md`. Part number and software version come from the image itself, so names are deterministic. Tests and tools use these paths only.
- Root symlinks into `images/`, for people browsing the corpus: `PART.bin` for a part number with one image, `PART-VERSION[-label].bin` for each image of a part with several. A part that gains a second image switches from the short name to full names, which is why tools don't use them. Made by `tools/corpus-links` and checked by `corpus-manifest -check`; no root links for definitions.
- `corpus.tsv` at the root, generated by `tools/corpus-manifest -corpus images` and committed. Columns: name, sha256, size, family, ident, oem, release, checksums, rsa, def (the image's definition, `defs/NAME.json`, or `-`). `corpus-manifest -check` (CI) also fails on a `def` that doesn't match `defs/` and on files in `defs/` not named after an image. Tests look files up by id or hash through the manifest, never by guessing paths.

### Policy

- Append-only: history is never rewritten, because every consumer pins a commit.
- Images are immutable: a changed image gets a new id. JSON files do change, through ordinary commits: hand fixes, and bulk regeneration when the schema or a reader improves. Bulk regeneration skips JSON with an edited stamp.
- Schema versions: every JSON names its schema version. xdfkit reads older versions, or the corpus is regenerated in one commit per schema bump. Consumers pin a corpus commit and an xdfkit version that agree.
- Canonical form (`jq -S .`, one value per line) keeps JSON diffs small and reviewable in git.
- Plain git, not Git LFS: about 100 MB of rarely changing binaries plus text JSON fits comfortably, and LFS would add a tool requirement and bandwidth quotas to every consumer's CI.
- Consumers add it with `shallow = true` in `.gitmodules`.

### Access: private repo

Decided 2026-10-07: `ecu-corpus` is a private GitHub repo, for now, and all development uses it. Consumer repos can be public: only the submodule URL and pinned commit are visible there.

- Everything works without access. A clone without access has an empty `corpus/`; corpus tests skip with a message naming this document, and nothing else depends on the corpus. Go modules never include submodule contents, so depending on xdfkit or me7-logger as a module is unaffected.
- Tests find the corpus at `XDFKIT_CORPUS`, default `corpus/` at the module root, and treat a missing `corpus.tsv` as "not available". In xdfkit, `internal/corpus` does this: `Open(t)` skips or fails, then images are looked up by name or SHA-256 and read from `images/NAME.bin`.
- `XDFKIT_REQUIRE_CORPUS=1` turns "not available" into a test failure. CI sets it on every run that has a corpus credential, so a revoked or expired credential fails the build instead of turning it silently green.
- Pull requests from forks get no secrets in GitHub Actions, so those runs leave `XDFKIT_REQUIRE_CORPUS` unset and skip corpus tests. Workflows must not use `pull_request_target` to get around this unless a maintainer gates the run.
- `.gitmodules` uses the absolute URL `https://github.com/nyetlabs/ecu-corpus.git`. A relative URL would also work now that the consumers are in `nyetlabs`, but would break for a fork or a consumer elsewhere. Developers who use SSH map it locally with `git config --global url.git@github.com:.insteadOf https://github.com/`.
- The submodule is set to `update = none` in `.gitmodules`, so a plain `git submodule update --init` doesn't try to fetch it and fail with an authentication error; developers with access check it out explicitly with `git submodule update --init --checkout corpus`. Unconfirmed: how `git clone --recurse-submodules` behaves with this setting.
- Developers get read access as collaborators on `ecu-corpus` and use their own GitHub login. No shared keys for people.
- Credential management must be automated: no long-lived personal tokens, no hand copying of keys between repos, and adding a consumer repo or rotating a credential is a scripted step. `GITHUB_TOKEN` can't read another private repo.

### CI credential: GitHub App

Decided 2026-10-07. Consumers: xdfkit, ecuxplot, ME7Sum and me7-logger, all in the `nyetlabs` organization (moved 2026-10-07), which also owns `ecu-corpus` and the app.

- One GitHub App owned by the organization, `nyetlabs-corpus-reader`, with only the Contents: read permission (plus the automatic Metadata: read), no webhook events, installed only on `ecu-corpus` (repository access "Only select repositories").
- The organization holds the app's Client ID as an Actions variable (`CORPUS_APP_CLIENT_ID`; `actions/create-github-app-token` v3 deprecates the numeric App ID) and the app's private key as an Actions secret (`CORPUS_APP_KEY`), visible to all its repos. Organization secrets don't reach repos outside the organization; such a consumer gets its own repo-level copy under the same names.
- Each CI run mints a token with `actions/create-github-app-token` (v3, `client-id`) (owner `nyetlabs`, repository `ecu-corpus`; it expires in about an hour), configures git to use it for the `ecu-corpus` URL only (`url.<token URL>.insteadOf`), then fetches the corpus with an explicit `git submodule update --init --checkout corpus` step (`--checkout` overrides `update = none`). The consumer repo itself is checked out with the default token. Templates: ecu-corpus `tools/consumer-ci/`.
- `tools/grant-ci-access` in `ecu-corpus` sets the variable and the secret with `gh variable set` and `gh secret set`: once at organization level with no arguments, or in the named repos for consumers outside the organization. A new consumer inside the organization needs nothing but the workflow templates.
- Rotation: generate a new private key in the app's settings (a web-only step on GitHub, unconfirmed whether an API exists), re-run the script (and for any repo-level consumers), then delete the old key. Revocation: delete the key or uninstall the app.
- Corpus bumps are automatic: a scheduled workflow in each consumer mints the app token, moves the submodule to the corpus head, and opens a pull request when it changed. Dependabot isn't used: its secrets are static (it can't mint the short-lived app token), and the workflow doesn't depend on the consumers being in the organization. Dependabot's organization-level private-repo access could reach `ecu-corpus` now; unconfirmed whether it would be simpler.

## Decided 2026-10-07

- Repo name: `ecu-corpus`.
- OEM evidence: `rsa`, `checksums` and `hand` qualify; a hand-set `release` = `production` gates a public corpus only.
- JSON converted from DAMOS or A2L goes in the shared corpus (private for now, but this decision assumed it could be public), even though it carries the originals' names, descriptions and conversions. The DAMOS/A2L originals themselves still never go in (hard decision above).
