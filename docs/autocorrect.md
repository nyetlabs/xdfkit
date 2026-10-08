# Autocorrect (`lint` and `fix`)

Design for fixing KP files in place. Evidence for the rules: scans of every EEPROM axis in the ecuxplot packs against their bins (2026-10-07), and the axis datasource section of `kp-format.md`.

Goal: fix the author's own KP files mechanically, instead of hand-editing each axis in WinOLS. This was blocked only by the lack of a KP writer, and it needs less than one: a patch-mode writer.

- Patch-mode writer:
  - The KP reader records the byte offset of every field it decodes: offsets in the file for v1, and offsets in the decompressed map block for v2.
  - Each fix changes only fixed-width fields that are already decoded: axis datasource enum, axis signed flag, axis address, and map/axis point count. Nothing is inserted or resized, so the rest of the file stays byte-identical.
  - v1: patch the bytes in place in a copy of the file.
  - v2: patch the decompressed map block, then re-zip it, keeping the original entry name, compression method and timestamps. The stage-1 round-trip gate also proves this, but patch mode can ship first. One WinOLS 2.24 check is needed: it must open a re-zipped, unmodified v2 file.
  - Unknown fields are never touched, so autocorrect doesn't depend on writing KP from scratch or on decoding the unknown fields.
- `xdfkit lint in.kp -i in.bin [--json findings.json]`: reports one finding per axis. Each finding has a rule id, a confidence, the evidence (raw values, plain and summed, the count byte) and a proposed fix. Axes shared by several maps are grouped by address.
- `xdfkit fix in.kp -i in.bin -o out.kp [--rules R1,R2] [--min-confidence high] [--only ID...] [--findings findings.json] [--dry-run]`:
  - Applies the proposed fixes. With `--findings`, it applies exactly the findings left in a JSON file you edited, which is the review workflow: lint to JSON, delete what you disagree with, then fix.
  - Never writes in place, and refuses to overwrite an existing file unless forced.
  - After writing, it re-reads the output and diffs the model against the input. Any change other than the intended fields is an error, and the output is deleted. It then re-lints and reports what is left.
- Rules:

| Rule | Detects | Fix | Confidence |
|---|---|---|---|
| R1 | "subtract" or "add" axis in an absolute-axis family (ME7: count byte only) whose plain values are monotonic | datasource to "EEPROM" | high |
| R2 | plain axis in a delta family (M3.x: [variable id][count] header matching the size) that is monotonic only when summed | datasource to "EEPROM, subtract" | high |
| R3 | only the opposite signedness gives a monotonic axis | toggle the signed flag | medium |
| R4 | the count byte before the axis disagrees with the axis size, and the count gives a monotonic axis | set the point count (map size follows) | medium; manual review |
| R5 | a valid header plus monotonic axis exists a few bytes away from the stored address | move the address | low; report only unless `--only` |
| R6 | all values identical, or no reading is monotonic | none; report only | n/a |
| R7 | ME7 only, using me7-logger's located maps (`integrations.md`): a map whose address matches no located map, a located map whose axis address or point count differs from the definition, or a located map missing from the definition | proposes the located axis address and point count; missing maps are reported as additions (patch mode can't insert) | medium; report only unless `--only` |

- Family detection: per image, by majority vote over axes that have a count byte, checking whether count bytes are preceded by plausible variable-id bytes. Override with `--family me7|m3`.
- R7 depends on the me7-logger integration and is implemented with it, after R1 to R6.
- R1 depends on whether WinOLS applies the running sum to "subtract" axes (unconfirmed; see the axis datasource section of `kp-format.md`). If it doesn't, R1 still fixes the label but becomes cosmetic.
