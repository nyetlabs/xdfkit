# Autocorrect (`lint` and `fix`)

Fixing KP files mechanically against the flash image. Implemented in `lint/` (DS/AX/XR/MP/RC codes below); exposed as `api.Lint` / `api.Fix`. Axis datasource formulas: `kp-format.md`. Finding field details: `go doc go.nyet.org/xdfkit/api`.

## Consumer contract

- `Finding.rule`: stable category code (two-letter prefix + two-digit serial).
- `Finding.confidence`: `high` / `medium` / `low` / `n/a` — how safe auto-fix is. Default `fix` applies high only. This is the only ordinal level xdfkit defines.
- `Finding.id`: `{rule}@0x{addr}:{n}` (points, or byte length for map findings).
- Hold / print policy for corpus tools stays in the consumer. me7parity holds on `DS01` and `DS03` (`integrations.md`).

| Prefix | Group |
| --- | --- |
| `DS` | axis-source (datasource / signedness) |
| `AX` | axis-geom (size / address) |
| `XR` | axis-report |
| `MP` | map |
| `RC` | me7-record |
| `MG` | merge (not emitted by `lint/`) |

## CLI

- `xdfkit lint -i in.bin [--family me7|m3] [--rules RC01,RC02] [--json findings.json] in.kp`
  - One finding per axis; one per map for MP01/RC01/RC02. Shared axes (same address and point count) are one finding. `--rules` filters.
  - Finding JSON is the caller interface: `id`, `rule`, `confidence`, `message`, evidence fields, optional `fix`, and `maps`. MP01 needs no image; DS/AX/XR and RC need the image. DS01/DS03/RC01/RC02 are ME7-only; DS02/AX01 are M3/M5-only.
  - RC01/RC02 attach an ME7 `record` (`model.md`) and a `cause` string (`values-pad`, `table-late`, `axis-pad`, `axis-width`, `shape`, `record-start`, or empty).
  - Input may be KP or model JSON. Exits 1 when there are findings.
- `xdfkit fix -i in.bin (-o out.kp | -n) [--rules DS01,DS02] [--min-confidence high|medium|low] [--only ID,...] [--findings findings.json] in.kp`
  - Applies proposed fixes (default: high confidence). `--findings` applies a reviewed lint JSON. Never writes in place; refuses overwrite without `--force`. `-n` reports without writing.
  - Before writing: re-parse the encoded output, compare, re-lint, and report what is left.
- `fix` mutates the decoded `kp.File` and re-encodes (`kp-format.md`, Round trip). Only decoded fixed-width fields change.

## Rules

| Rule | Detects | Fix | Confidence |
|---|---|---|---|
| DS01 | "subtract" or "add" axis in an absolute-axis family (ME7) whose plain values are monotonic | datasource to "EEPROM" | high |
| DS01 | the same, monotonic only with the opposite signedness | datasource to "EEPROM" and toggle signed | medium |
| DS02 | plain axis in a delta family (M3/M5) that is not monotonic but, as "EEPROM, subtract", increases from 0 or more | datasource to "EEPROM, subtract" | high |
| DS03 | ME7 plain axis where only the opposite signedness is monotonic | toggle signed | medium |
| AX01 | M3/M5 8-bit axis whose previous byte disagrees with the size (plausible count 1–24 or declared data header; bare 0/255 is not a header) | set point count when that size is monotonic; else report only | medium when fixed, else n/a |
| AX02 | 8-bit axis address not a valid reading, but a count and monotonic axis sit within ±4 bytes | move axis address; only with `--only` or `--min-confidence low` | low |
| XR01 | all values identical; or no monotonic reading (ME7); or add/subtract that does not increase from 0 as subtract (M3/M5) | none; report only | n/a |
| MG01 | ME7 only, against me7info-located maps (origin `auto`): address or axis mismatch, or missing located map | `xdfkit merge --update` (`design.md`); not a lint rule | medium; report only |
| MP01 | map end other than start + byte length | set end address | high |
| RC01 | ME7: 16- or 32-bit integer cells at an odd address | move start or axis per `cause`; no fix for `shape` / empty cause; only with `--only` or `--min-confidence low` | low when fixed, else n/a |
| RC02 | ME7: curve/map without image axes whose data starts with its own counts then monotonic axes (`cause` `record-start`) | move start to record values address; only with `--only` or `--min-confidence low` | low |

- Only image-reading integer axes with at least two points are checked; float axes skipped. MP01 checks every map without the image; RC01 every integer map of 16+ bits; RC02 every integer curve/map without image axes.
- Family: from the Bosch ident in the image (`…/ME7…` → ME7; `…/M3…` / `…/M5…` → delta). Images without one need `--family`.
