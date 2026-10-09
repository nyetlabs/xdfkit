# Edit stamp and XDF metadata file

Spec items added 2026-10-07:

- Every JSON and XDF output carries a stamp that tells whether its data has been hand edited since xdfkit wrote it. Files are written in deterministic `jq` output form, `jq -S .` (sorted keys, 2-space indent per `.cursor/rules/json.mdc`, decided 2026-10-07). The stamp holds two digests, one over that text and one over RFC 8785 (see Stamp).
- XDF output comes with a metadata JSON file holding the parts of the model that XDF can't represent.

## Canonical text

- `C(doc)` is exactly what `jq -S .` prints for `doc`: 2-space indent, object keys sorted at every level (by code point), UTF-8 without escaping non-ASCII, one trailing newline.
- xdfkit computes `C` itself, with no dependency on jq at runtime. Tests pipe every golden output through real jq and require identical bytes.
- xdfkit writes its JSON files in canonical form, so `jq -S .` of an unedited file reproduces it byte for byte, and diffs between outputs stay minimal.
- Differences between Go's `encoding/json` and `jq` (jq 1.8.1, checked 2026-10-07), which the canonical printer must handle:
  - Go escapes `<`, `>`, `&` as `\u003c` etc. unless `SetEscapeHTML(false)`; jq prints them raw. Go also always escapes U+2028/U+2029; jq doesn't.
  - Exponents: Go writes `1e-7`, `1e+21`; jq 1.8 writes `1E-7`, `1E+21`. Older jq (1.6) prints numbers differently again.
  - Plain decimals and integers (`0.1`, `0.0078125`, `655.35`, `-30`, `0.3333333333333333`) are identical.
- Number rule: the writer spells every number exactly as jq 1.8 reprints it, by Go type (see Typed numbers): integers as integer literals, floats as shortest round-trip decimals, with `.0` on whole values (`-30.0`, `1000000000000000000000.0`). Magnitudes from 1e-6 up are plain decimals, including those at 1e21 and above, which jq keeps as plain digits. Magnitudes below 1e-6 use jq's exponent form (`4.7E-7`, `-7.31383E-307`), because jq 1.8 rewrites a plain `0.00000047` as `4.7E-7`. Addresses stay below 2^53.
- Real cases (2026-10-07): map factors `4.7E-7` (FMDWAT) and `5.96E-8` (FVERZDYN) in five ecuxplot packs, and the offset `-7.31383E-307` (probably junk) on DELTATA in three. Stamps over files containing such numbers assume jq 1.8's spelling; other jq versions may print them differently (the RFC 8785 digest is unaffected). Whether to accept this, or pin the jq version in the canon name, is still open. Implemented in `canon/`; tests compare against real jq.
- Struct fields are emitted sorted by JSON name, not in Go declaration order (Go's encoder sorts map keys but not struct fields).

## Typed numbers

Decided and implemented 2026-10-07 (`canon/typed.go`: `canon.Marshal`, `canon.MarshalStamped`, `canon.Unmarshal`). JSON has one number type, which causes three problems: Go's decoder rejects `16.0` for an integer field, a whole-number float is written `1` and looks like an integer (YAML infers int or float from spelling), and the `jq -S .` digest treats `1` and `1.0` as different while RFC 8785 doesn't.

- `canon` gets its own type-guided JSON coding: Go value to generic tree and back, by reflection over the Go types (like the KP codec), instead of going through `encoding/json`'s typed path. The CLI reads and writes JSON only through it.
- Writing:
  - Integer kinds: integer literals.
  - Floats: shortest round-trip digits, jq 1.8's exponent form below 1e-6, and whole values always with `.0` (`1.0`, `-30.0`, `-0.0`), so the type is visible in the file. jq 1.7+ keeps `1.0` as written, so `jq -S .` stays a fixed point; RFC 8785 is unaffected.
  - Types with their own JSON form (`kp.Hex`, the enums) still go through `MarshalJSON`; numbers in that output, and `json.Number` values, are respelled to jq's form.
  - Struct tags follow `encoding/json`: names, `-`, `omitempty`, `omitzero`, and fields promoted from untagged embedded structs. `[]byte` is base64.
- Reading:
  - Integer kinds accept any literal whose exact value is a whole number in range (`16`, `16.0`, `1.6E1`), checked exactly with `math/big`, so there is no rounding above 2^53. Anything else is an error naming the field's path, as in `project.maps[3].cols: 16.5 is not an integer`.
  - Floats accept any number.
  - Unknown fields are errors, except the top-level `stamp`. Types with `UnmarshalJSON` get their subtree. Arrays must have exactly the Go array's length. Key names match exactly (no case folding).
- Effects:
  - A respelled number still shows as `mixed` (the jq digest flags it), but the file always converts.
  - YAML goes through the same typed layer (decided 2026-10-08): it is a syntax layer over the canonical JSON, converted node for node with each number's spelling kept, so the int/float rules and the stamp apply to it unchanged (`canon/yaml.go`: `canon.JSONToYAML`, `canon.YAMLToJSON`). Strings that YAML would read as another type (`"0x1F"`, `"true"`) are quoted. On input, YAML number spellings that JSON lacks (`0x1F`, `1_000`) are normalized, `.inf` and `.nan` are errors, and timestamps are read as strings.

## Stamp

- Two digests, decided 2026-10-07, kept side by side until experience shows which one to keep. Both are SHA-256 over the document without its `stamp`, written `sha256:<hex>` (the OCI digest form):
  - `RFC8785`: the JSON Canonicalization Scheme (RFC 8785), the common standard for hashing JSON: compact, sorted keys, numbers compared by value and printed as ECMAScript does. Computed with `github.com/gowebpki/jcs`.
  - `jq -S .`: `C(doc)` above, the same text xdfkit writes.
- In JSON (full model and metadata files): a top-level `stamp` object, placed wherever the key sort puts it:

```json
"stamp": {
  "digests": [
    {
      "canon": "RFC8785",
      "digest": "sha256:9cdc..."
    },
    {
      "canon": "jq -S .",
      "digest": "sha256:4c8a..."
    }
  ],
  "tool": "xdfkit 0.1.0"
}
```

- The `jq -S .` digest can be checked with jq alone; the RFC 8785 one needs a JCS tool (a digest command in xdfkit is planned):

```sh
jq -S 'del(.stamp)' file.json | shasum -a 256
jq -r '.stamp.digests[] | select(.canon == "jq -S .") | .digest' file.json
```

- `xdfkit verify file` checks every digest it knows and prints the overall result with each digest's: `clean`, `edited`, `mixed` (the digests disagree), `unknown` (no digest in a supported form) or `unstamped`. Any xdfkit reader warns when it loads a file that isn't clean.
- What counts as an edit: any value change, adding or removing keys, and reordering array elements. Whitespace, indentation and object key order count under neither digest. Respelling a number (`1` as `1.0`) counts under `jq -S .`, because jq 1.7+ keeps the literal, but not under RFC 8785, which compares numbers by value; such an edit shows as `mixed`. Patterns like this are what running both digests is meant to surface.
- The stamp detects hand edits, not tampering: anyone can recompute it. It isn't a signature.

### XDF stamp

- An XDF has no JSON of its own, so its digest covers the XDF view: the model JSON that xdfkit's XDF reader builds from the XDF elements alone (`xdf.Read` without a metadata file), without object keys, which the reader derives from the titles of all objects. That excludes comments, the embedded model block, the stamp line itself, and label values (the model has no field for them), and objects are sorted by uniqueid. Digests: the same two forms as the JSON stamp, over the XDF view.
- This catches real edits made in TunerPro (values, addresses, scaling, axes, added or deleted tables). It ignores TunerPro reformatting, attribute order and table order.
- Where it is stored:
  - In the metadata file: `xdf.digests` (both forms, for the whole view). Implemented.
  - Best effort, in the XDF itself (not written yet; the reader already leaves it out of the view): one line `xdfkit-stamp: sha256:<digest>` in the `XDFHEADER` description, a field TunerPro shows and preserves. That line is excluded from the XDF view. Confirmed 2026-10-07: TunerPro shows the description as two lines and keeps the stamp line through a save (writing the line break as `&#010;`; it writes other description line breaks as `&#013;&#010;`, so readers accept both).
- A tuner subset (`model.md`, `provenance.subset`) adds the line `xdfkit-subset: tuner sha256:<table>` to the same description. The reader takes it out of the project name and into `provenance.subset`, so a tuner XDF read without its metadata file, or with another XDF's, still reads as a subset.

## Metadata file

Implemented 2026-10-08 (`xdf/meta.go`: `xdf.Meta`, `xdf.Read`).

- Written next to every XDF output file: `name.xdf` and `name.meta.json` (see `naming.md`). XDF written to stdout gets none. API callers get it as `ConvertResponse.Meta` and pass it back as `ConvertRequest.Meta`.
- Contents: the difference between the model and the XDF view of the XDF written from it. It is the residue, not the full model. The model carries no KP residue (`model.md`), so neither does the metadata file; KP output from an XDF takes the undecoded KP fields from a template or the defaults, as from JSON.

```json
{
  "model": {"categories": [...], "provenance": {...}},
  "objects": {
    "0x1": {"categories": [2], "id": "KFLDRQ2 (AR 27C02)", "inverse": true, "key": "KFLDRQ2 (AR 27C02)"}
  },
  "schema": "xdfkit-xdf-meta/1",
  "stamp": {...},
  "xdf": {
    "digests": [{"canon": "RFC8785", "digest": "sha256:..."}, {"canon": "jq -S .", "digest": "sha256:..."}]
  }
}
```

- `model`: a JSON Merge Patch (RFC 7396) from the view without its objects to the model without its objects: provenance, categories (KP folder ids and order; the XDF numbers categories by sorted name), project fields the XDF lost.
- `objects`: per object, keyed by its XDF `uniqueid`, the merge patch from its view to the model object, left out when empty. Typical entries: the key, the full id (the XDF title has only its first word), the comment (merged into the XDF description), category ids, `inverse`, `value.description`, the display base, axis `mirror`, `header` and `signature`, the storage of "subtract" axes written as labels, and precision the writer limited to six digits. Keyed by uniqueid rather than title because titles repeat; TunerPro keeps uniqueids through a save (8D0907551M, observed 2026-10-07).
- `xdf`: the digests of the view (see XDF stamp).
- Its own `stamp`.
- Reconstruction (`xdf.Read`): model = XDF view merged with metadata, by uniqueid.
  - Digest matches `xdf.digests`: the merge is exact, so XDF plus metadata reproduces the model and JSON to XDF to JSON is lossless (tested on every archived pack, with and without image).
  - Digest differs (edited in TunerPro): warning. The project takes the `model` patch over the XDF's header, and XDF categories missing from the metadata are added. Each object takes the XDF's values, and keeps the metadata's for what XDF can't express (key, `inverse`, `value.description`, display settings, axis `mirror`, `header`, `signature`, axis storage the XDF can't show) and where the metadata's value lowers to what the XDF holds (id, description and comment when the title and description are unchanged; category, shape, units, conversion, precision). Unedited objects therefore come out as before. A warning names each object whose metadata fields were not applied; it can't name which XDF values changed, because the metadata file doesn't hold the original view.
  - Objects in the metadata but missing from the XDF are dropped with a warning; objects new in the XDF (or with a repeated uniqueid) have no metadata, get a key from their title, and are reported.
  - Metadata stamp doesn't match: someone hand edited the metadata file. Warn and use it anyway.
  - No metadata file: the plain XDF view, with provenance format `xdf`.
- The XDF view: title as id, description as description, `CATEGORYMEM` as categories, cells from `EMBEDDEDDATA` (type flags: signed, little endian, float), the equation as factor and offset (any expression linear in X, or factor/X + offset for KP's reciprocal; anything else reads as X with a warning), `decimalpl` (default: `DEFAULTS sigdigits`) as precision, output type 3 as base 16 and the others as base 10. Table axes with a location are image axes; label axes are ordinal, and dropped when they have at most one label. Elements other than tables and constants (`XDFFLAG`, `XDFPATCH`, `XDFFUNCTION`) are ignored with a warning. Input that isn't valid UTF-8 is read as Windows-1252.

## Relation to the embedded model block

- The metadata file is the durable, primary copy of the residue. The comment block in `xdf-embedding.md` doesn't survive a TunerPro save (confirmed).
- Decided 2026-10-07: the embedded block is optional (`--embed`, off by default) and carries the same payload as the metadata file (residue plus digests, XML encoded), not a second full model. Keeping one source of truth avoids the two copies disagreeing.

## Decided

- Metadata file name: `name.meta.json` next to `name.xdf` (2026-10-07; see `naming.md`).
