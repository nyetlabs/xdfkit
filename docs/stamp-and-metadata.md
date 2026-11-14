# Edit stamp and XDF metadata file

Every JSON and XDF output carries a stamp that tells whether its data was hand edited since xdfkit wrote it. XDF output comes with a metadata file holding what XDF can't represent. JSON is written in `jq -S .` form. Default output names replace the input's extension (`foo.kp` → `foo.xdf`, `foo.json`). The XDF metadata file is `foo.meta.json` next to `foo.xdf`.

## Canonical text

- `C(doc)` is exactly what `jq -S .` prints: 2-space indent, keys sorted by code point, UTF-8 without escaping non-ASCII, one trailing newline.
- `canon/` computes `C` without calling jq; tests require identical bytes from real jq.
- Numbers follow Go types (Typed numbers). Exponents below 1e-6 use jq 1.8's spelling (`4.7E-7`). Hand-checking the `jq -S .` digest needs jq 1.8+. RFC 8785 is unaffected.
- Addresses stay below 2^53.

## Typed numbers

Implemented in `canon/typed.go`. Tags and YAML edge cases: `go doc go.nyet.org/xdfkit/canon`.

- Writing: integers as integer literals; floats as shortest round-trip decimals with `.0` on whole values; jq 1.8 exponent form below 1e-6.
- Reading: integer fields accept any exact whole literal in range; other values error with the JSON path. Floats accept any number. Unknown fields are errors, except top-level `stamp`.
- A respelled number (`1` as `1.0`) is `mixed` under the jq digest but still converts. YAML keeps number spellings node for node.

## Stamp

- Two digests, SHA-256 over the document without its `stamp`, written `sha256:<hex>`:
  - `RFC8785` via `github.com/gowebpki/jcs`
  - `jq -S .`: `C(doc)` above
- Top-level `stamp` on JSON (model and metadata):

```json
"stamp": {
  "digests": [
    {"canon": "RFC8785", "digest": "sha256:9cdc..."},
    {"canon": "jq -S .", "digest": "sha256:4c8a..."}
  ],
  "tool": "xdfkit 0.1.0"
}
```

```sh
jq -S 'del(.stamp)' file.json | shasum -a 256
jq -r '.stamp.digests[] | select(.canon == "jq -S .") | .digest' file.json
```

- `xdfkit verify file` prints `clean`, `edited`, `mixed`, `unknown`, or `unstamped`; exits 1 when not clean. `--status` prints only the word. `edited` loads without warning; `mixed` and `unknown` need a look.
- An edit is any value change, added or removed key, or reordered array. Whitespace and key order count under neither digest. Number respelling counts only under `jq -S .` (`mixed`).
- The stamp detects hand edits, not tampering.

### XDF stamp

- Digests cover the XDF view: model JSON from XDF elements alone (`xdf.Read` without metadata), without object keys (from titles). Excludes comments, stamp lines, and label values. Objects sorted by uniqueid.
- Stored in the metadata file as `xdf.digests`. The XDF itself has no stamp line. A header description line starting with `xdfkit-stamp:` is left out of the view.
- A tuner subset (`model.md`) adds `xdfkit-subset: tuner sha256:<table>` to that description; the reader restores `provenance.subset`.

## Metadata file

Implemented in `xdf/meta.go` (`xdf.Meta`, `xdf.Read`).

- Written next to every XDF: `name.xdf` and `name.meta.json`. Stdout gets none. API: `ConvertResponse.Meta` / `ConvertRequest.Meta`.
- Contents: the difference between the model and the XDF view (residue, not the full model). KP residue stays out (`model.md`).

```json
{
  "model": {"categories": [...], "provenance": {...}},
  "objects": {
    "0x1": {"categories": [2], "id": "KFLDRQ2", "inverse": true, "key": "KFLDRQ2"}
  },
  "schema": "xdfkit-xdf-meta/1",
  "stamp": {...},
  "xdf": {
    "digests": [{"canon": "RFC8785", "digest": "sha256:..."}, {"canon": "jq -S .", "digest": "sha256:..."}]
  }
}
```

- `model`: JSON Merge Patch (RFC 7396) from the view without objects to the model without objects.
- `objects`: per object, keyed by XDF `uniqueid`, the merge patch from its view to the model object (omitted when empty). Typical residue: key, full id, comment, category ids, `inverse`, `vislevel`, `value.description`, display base, axis `backwards` / `header` / `signature`, add/subtract storage, precision capped at six digits.
- `xdf`: digests of the view. Own `stamp`.

### Reconstruction (`xdf.Read`)

- Digest matches: exact merge; XDF plus metadata reproduces the model.
- Digest differs (edited in TunerPro): warn; project takes the `model` patch; each object prefers XDF values and keeps metadata for what XDF cannot express. A warning names objects whose metadata fields were not applied.
- Objects only in metadata: dropped with a warning. New XDF objects: key from title.
- Metadata stamp mismatch: warn and use it anyway.
- No metadata: plain XDF view, provenance format `xdf`.

### XDF view

- Title → id; description → description; `CATEGORYMEM` → categories.
- Cells from `EMBEDDEDDATA`; equation → factor/offset (linear in X, or factor/X + offset; else X with a warning).
- `decimalpl` (default `DEFAULTS sigdigits`) → precision; output type 3 → base 16, else 10.
- Missing `mmedtypeflags` on a wider cell: endian from `DEFAULTS lsbfirst`, else little-endian.
- Missing `mmedaddress` → address 0. `min`/`max` kept only when they differ from the writer's invented range.
- Image axes: location present; label axes: ordinal (dropped at ≤1 label). Non-UTF-8 input read as Windows-1252.

### Categories and alternates

One TunerPro table has one category and one equation. A map in several categories, or with an alternate (`model.md`), is one table per category and per alternate. Primary tables keep uniqueid `0x1` onward; extras get `xdfkit-fold: 0xPRIMARY` in the description. Metadata pairs only primaries. Read with metadata drops extras; without it they stay (`merge --smart` folds them).

### Live helpers (add / subtract)

For an unsigned 8/16/32-bit add or subtract axis that is not reciprocal, the writer emits a 1-column helper table at the axis address, linked with `embedinfo type="3"`:

- Subtract: last row `(2^bits - X) * factor + offset`; earlier rows `CELL(ROW()+1;0;FALSE) - factor * X`.
- Add: first row `factor * X + offset`; later rows `CELL(ROW()-1;0;FALSE) + factor * X`.

Rows numbered from 1. `--static-axes` writes static labels instead (needed for TunerPro before 5.00.8414). Signed, float, reciprocal add/subtract, and backwards use labels. The reader leaves helpers out of the model; the metadata file keeps the axis storage. Formulas: `kp-format.md`.

## Embedded block (not implemented)

Optional `--embed` would put the same payload as the metadata file in an XML comment after `XDFHEADER`. The metadata file remains the durable primary copy; a TunerPro save drops the comment. Not implemented.
