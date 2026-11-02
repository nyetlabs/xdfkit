# Autocorrect (`lint` and `fix`)

Fixing KP files mechanically against the flash image. Implemented in `lint/` (R1–R6, R8–R10); exposed as `api.Lint` / `api.Fix`. Axis datasource formulas: `kp-format.md`. Finding field details: `go doc go.nyet.org/xdfkit/api`.

## CLI

- `xdfkit lint -i in.bin [--family me7|m3] [--rules R9,R10] [--json findings.json] in.kp`
  - One finding per axis; one per map for R8–R10. Shared axes (same address and point count) are one finding. `--rules` filters.
  - Finding JSON is the caller interface: `id`, `rule`, `confidence`, `message`, evidence fields, optional `fix`, and `maps`. R8 needs no image; R1–R6 and R9/R10 need the image. R1/R3/R9/R10 are ME7-only; R2/R4 are M3/M5-only.
  - R9/R10 attach an ME7 `record` (`model.md`) and a `cause` string (`values-pad`, `table-late`, `axis-pad`, `axis-width`, `shape`, `record-start`, or empty).
  - Input may be KP or model JSON. Exits 1 when there are findings.
- `xdfkit fix -i in.bin (-o out.kp | -n) [--rules R1,R2] [--min-confidence high|medium|low] [--only ID,...] [--findings findings.json] in.kp`
  - Applies proposed fixes (default: high confidence). `--findings` applies a reviewed lint JSON. Never writes in place; refuses overwrite without `--force`. `-n` reports without writing.
  - Before writing: re-parse the encoded output, compare, re-lint, and report what is left.
- `fix` mutates the decoded `kp.File` and re-encodes (`kp-format.md`, Round trip). Only decoded fixed-width fields change.

## Rules

| Rule | Detects | Fix | Confidence |
|---|---|---|---|
| R1 | "subtract" or "add" axis in an absolute-axis family (ME7) whose plain values are monotonic | datasource to "EEPROM" | high |
| R1 | the same, monotonic only with the opposite signedness | datasource to "EEPROM" and toggle signed | medium |
| R2 | plain axis in a delta family (M3/M5) that is not monotonic but, as "EEPROM, subtract", increases from 0 or more | datasource to "EEPROM, subtract" | high |
| R3 | ME7 plain axis where only the opposite signedness is monotonic | toggle signed | medium |
| R4 | M3/M5 8-bit axis whose previous byte disagrees with the size (plausible count 1–24 or declared data header; bare 0/255 is not a header) | set point count when that size is monotonic; else report only | medium when fixed, else n/a |
| R5 | 8-bit axis address not a valid reading, but a count and monotonic axis sit within ±4 bytes | move axis address; only with `--only` or `--min-confidence low` | low |
| R6 | all values identical; or no monotonic reading (ME7); or add/subtract that does not increase from 0 as subtract (M3/M5) | none; report only | n/a |
| R7 | ME7 only, against me7info-located maps (origin `auto`): address or axis mismatch, or missing located map | `xdfkit merge --update` (`design.md`); not a lint rule | medium; report only |
| R8 | map end other than start + byte length | set end address | high |
| R9 | ME7: 16- or 32-bit integer cells at an odd address | move start or axis per `cause`; no fix for `shape` / empty cause; only with `--only` or `--min-confidence low` | low when fixed, else n/a |
| R10 | ME7: curve/map without image axes whose data starts with its own counts then monotonic axes (`cause` `record-start`) | move start to record values address; only with `--only` or `--min-confidence low` | low |

- Only image-reading integer axes with at least two points are checked; float axes skipped. R8 checks every map without the image; R9 every integer map of 16+ bits; R10 every integer curve/map without image axes.
- Family: from the Bosch ident in the image (`…/ME7…` → ME7; `…/M3…` / `…/M5…` → delta). Images without one need `--family`.
