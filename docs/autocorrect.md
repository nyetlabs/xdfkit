# Autocorrect (`lint` and `fix`)

Fixing KP files mechanically, instead of hand-editing each axis in WinOLS. Evidence for the rules: scans of every EEPROM axis in the ecuxplot packs against their bins (2026-10-07), and the axis datasource section of `kp-format.md`. Implemented in `lint/` (rules R1 to R6 and R8, of which R5 and the R4 fix are not yet implemented), exposed as the `lint` and `fix` methods of `api/` and the CLI commands below. Tests: the findings on the archived ecuxplot packs with their corpus images, and synthetic fixtures that need no corpus (`testdata/lint/cases.json`, `lint/fixture_test.go`): made-up images holding a Bosch ident and raw axis values, with maps built through the model, covering R1 (both confidences), R2, R3, R4, R6, R8 and an axis shared by two maps; every fix is applied, re-encoded and re-linted.

- Writer: `fix` changes the decoded `kp.File` and re-encodes it with the KP writer, which round-trips every pack (v1 byte-identical, v2 identical except for the re-deflated zip; `kp-format.md`, Round trip). Each fix changes only fixed-width fields that are already decoded (axis datasource, axis signed flag, map end address), so nothing else in the file moves and unknown fields are carried through as raw bytes. An earlier design patched bytes at recorded offsets; the round-trip writer makes that unnecessary. WinOLS 2.24 opens re-encoded v2 files and fixed files cleanly (confirmed 2026-10-07).
- `xdfkit lint -i in.bin [-family me7|m3] [-json findings.json] in.kp`:
  - Reports one finding per axis, and one per map for R8. Axes shared by several maps (same address and point count) are one finding listing every map axis.
  - Each finding has an ID (`RULE@0xADDR:POINTS`, for example `R1@0x1504e:4`; for R8 the map's start address and byte length), a rule, a confidence, a message, the evidence (raw values, the points WinOLS would show for an "EEPROM, subtract" axis, the values with the opposite signedness when that matters, the count byte before 8-bit axes) and the proposed fix.
  - The input may be the KP file or its model JSON (`model.md`). Exits 1 when there are findings.
- `xdfkit fix -i in.bin (-o out.kp | -n) [-rules R1,R2] [-min-confidence high|medium|low] [-only ID,...] [-findings findings.json] in.kp`:
  - Applies the proposed fixes, by default those of high confidence.
  - With `-findings`, it applies the fixes left in a lint JSON file, which is the review workflow: lint to JSON, delete what you disagree with, then fix. `-only`, `-rules` and `-min-confidence` filter that list too. A finding whose axis no longer has the same address and point count is an error, so a stale file can't change the wrong axis.
  - Never writes in place, and refuses to overwrite an existing file unless `-force` is given. `-n` reports without writing.
  - Before writing, it parses the encoded output back and compares it with the fixed file; any difference is an error and nothing is written. It then re-lints the output and reports what is left.
- Rules:

| Rule | Detects | Fix | Confidence |
|---|---|---|---|
| R1 | "subtract" or "add" axis in an absolute-axis family (ME7) whose plain values are monotonic | datasource to "EEPROM" | high |
| R1 | the same, monotonic only with the opposite signedness | datasource to "EEPROM" and toggle the signed flag | medium |
| R2 | plain axis in a delta family (M3.x, M5.x) that is not monotonic but, read as "EEPROM, subtract", increases from 0 or more | datasource to "EEPROM, subtract" | high |
| R3 | ME7 plain axis where only the opposite signedness gives a monotonic axis | toggle the signed flag | medium |
| R4 | M3.x/M5.x 8-bit axis whose count byte disagrees with the axis size | report only; planned: set the point count (map size follows) when the count gives a monotonic axis, medium, manual review | n/a |
| R5 | a valid header plus monotonic axis exists a few bytes away from the stored address | planned: move the address, low, report only unless `-only` | |
| R6 | all values identical; or no reading is monotonic (ME7); or a "subtract" or "add" axis that, read as "subtract", doesn't increase from 0 or more (M3.x/M5.x) | none; report only | n/a |
| R7 | ME7 only, using me7-logger's located maps (`integrations.md`): a map whose address matches no located map, a located map whose axis address or point count differs from the definition, or a located map missing from the definition | planned: propose the located axis address and point count; missing maps are reported as additions | medium; report only unless `-only` |
| R8 | map end address other than start + byte length (WinOLS 2.24 writes that value and recomputes it on export; `kp-format.md`, WinOLS 2.24 exports) | set the end address | high |

- Only axes whose datasource reads the image and that have at least two integer points are checked; float axes are skipped. R8 checks every map and doesn't read the image.
- Family: from the Bosch identification string in the image (`40/1/ME7.1/5/...` is ME7, `5655/1/M3.82/...` and `9655/1/M5.92/...` are delta). Images without one need `-family`.
- R7 depends on the me7-logger integration and is implemented with it.
- R1 changes what WinOLS displays: WinOLS shows a "subtract" axis as 256 minus the sum of the remaining raw values (confirmed in WinOLS 2.24; see the axis datasource section of `kp-format.md`), so a mislabelled absolute axis shows wrong values until fixed.
