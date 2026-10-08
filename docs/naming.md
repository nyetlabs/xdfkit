# File naming convention

Decided 2026-10-07 for the shared corpus and xdfkit outputs: image names from the image's own part number and version, lowercase labels, an owner in every definition name, and SHA-256 digits in place of a missing version. Original files kept for regression tests keep their original names; their identity is their SHA-256.

## Character rules

- ASCII letters, digits, `-` and `.` only. No spaces, no underscores.
- `.` only separates stem, label and extension, so the stem is everything before the first `.`.
- Extensions are lowercase (`.bin`, `.json`).
- Names must be unique ignoring case (macOS and Windows file systems are case-insensitive).

## Images

`PART-VERSION[-label].bin`

- `PART`: the VAG part number as stored in the image (`8D0907551M`, `4D1907558`, `006410010A0`), uppercase, never containing `-`.
- `VERSION`: the software version as stored in the image, 3 or 4 characters, never containing `-`: usually digits (`0002`), sometimes not (`V003`, `D01`, `EU3` in a tuned 8N0906018CB, `4000` in 8D0907551AA, `0000` in the Spyker image). Always present, even when only one version of a part is known, so names don't change when a second version turns up.
- This matches what the repos already do: every existing `-NNNN` suffix (`8D0907551M-0002`, `4D1907558-0004`, `4B0906018DQ-0060`) equals the version stored in that image. Unsuffixed files carry a version too (ecuxplot's `8D0907551M.bin` is `8D0907551M-0002`).
- `label`: only for images that differ from what the part and version identify (modified, corrected, tuned, rebuilt from hex). The shared corpus is OEM only (`corpus.md`), so corpus names never have a label; labels are for modified images kept elsewhere (xdfkit `testdata/`, the source repos). Lowercase kebab-case: `8D0907551M-0002-16b-kfzw`, `8D0907551M-0002-5120`, `4Z7907551AA-0010-disable-p1681`, `4Z7907551R-ca17aaeb-ols-corrected`.
- Parsing: split at the first two `-`; the rest is the label.
- No identification in the image (wiped, unusual layout, or not a VAG image): the part number is assigned by hand, and the version is the first 8 hex digits of the image's SHA-256 (`4Z7907551R-f5ec43d3`). That keeps names unique and deterministic without inventing a version. The manifest marks such names `manual`.
- Checksum status, provenance and family aren't in the name; they're manifest columns. No `broken/` or family directories.
- The generator (ecu-corpus `tools/corpus-manifest`) derives names from the image contents; ecu-corpus `tools/corpus-draft.tsv` has the proposed name for every image surveyed at seeding, OEM or not.

## Definitions (JSON)

`IMAGE-STEM.OWNER.json`

- `IMAGE-STEM`: the stem of the image it describes.
- `OWNER`: the project or person maintaining that definition line, lowercase kebab-case: `8D0907551M-0002.ecuxplot.json` (from the KP), `8D0907551M-0002.me7-logger.json` (from the hand XDF). Always present, so there is no "primary" definition to argue about, and edits have a clear owner.
- No dates in names: history is in git. ecuxplot's dated KP copies (`8D0907551M-20261006`) become commits of one JSON.
- Tool outputs that aren't curated definitions (me7info's generated maps) don't go in the corpus (see `corpus.md`).

## xdfkit outputs

- Default output names replace the input's extension: `foo.kp` becomes `foo.json`, `foo.xdf`, `foo.yaml`.
- The XDF sidecar is `foo.meta.json` next to `foo.xdf` (confirmed 2026-10-07). `meta` is reserved and can't be used as an owner.
- WinOLS script output: extension to be confirmed from the help file's examples.

## Hand overrides

- Names, OEM status and release status that can't be computed live in ecu-corpus `corpus-overrides.tsv` (sha256, name, oem, release, note), read by `tools/corpus-manifest -overrides`. `-` keeps the computed value.
