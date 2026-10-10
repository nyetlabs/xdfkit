# Edit stamp and XDF metadata file

Every JSON and XDF output carries a stamp that tells whether its data was hand edited since xdfkit wrote it, and XDF output comes with a metadata file holding what XDF can't represent. JSON is written in `jq -S .` form.

## Canonical text

- `C(doc)` is exactly what `jq -S .` prints for `doc`: 2-space indent, object keys sorted at every level (by code point), UTF-8 without escaping non-ASCII, one trailing newline.
- `canon/` computes `C` itself (no jq at runtime); tests require identical bytes from real jq. Unedited files are therefore a `jq -S .` fixed point.
- Unlike Go's `encoding/json`, jq doesn't escape `<`, `>`, `&`, U+2028 or U+2029, writes exponents as `1E-7` (jq 1.8; 1.6 differs again), and struct fields come out sorted like map keys.
- Numbers are spelled as jq 1.8 reprints them, by Go type (Typed numbers): integers as integers, floats as shortest round-trip decimals with `.0` on whole values (`-30.0`, `1000000000000000000000.0`), plain decimals from 1e-6 up, and jq's exponent form below (`4.7E-7`), since jq 1.8 rewrites `0.00000047` that way. Addresses stay below 2^53.
- Real cases: factors `4.7E-7` (FMDWAT) and `5.96E-8` (FVERZDYN) in five ecuxplot packs, offset `-7.31383E-307` (junk, unconfirmed) on DELTATA in three. Their `jq -S .` digest assumes jq 1.8's spelling (RFC 8785 is unaffected).

## Typed numbers

Implemented in `canon/typed.go` (`Marshal`, `MarshalStamped`, `Unmarshal`). JSON has one number type: Go rejects `16.0` for an integer field, a whole float written `1` looks like an integer (YAML infers type from spelling), and the `jq -S .` digest tells `1` from `1.0` while RFC 8785 doesn't. So `canon` codes JSON by reflection over the Go types (like the KP codec), and the CLI reads and writes JSON only through it.

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
  - YAML (`canon/yaml.go`) is converted node for node to and from the canonical JSON with number spellings kept, so these rules and the stamp apply unchanged. Strings that YAML would read as another type (`"0x1F"`, `"true"`) are quoted. On input, YAML number spellings that JSON lacks (`0x1F`, `1_000`) are normalized, `.inf` and `.nan` are errors, and timestamps are read as strings.

## Stamp

- Two digests, kept side by side until experience shows which one to keep. Both are SHA-256 over the document without its `stamp`, written `sha256:<hex>` (the OCI digest form):
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
- An edit is any value change, added or removed key, or reordered array. Whitespace and key order count under neither digest. Respelling a number (`1` as `1.0`) counts only under `jq -S .` (jq 1.7+ keeps literals) and shows as `mixed`.
- The stamp detects hand edits, not tampering: anyone can recompute it. It isn't a signature.

### XDF stamp

- An XDF has no JSON of its own, so its digest covers the XDF view: the model JSON that xdfkit's XDF reader builds from the XDF elements alone (`xdf.Read` without a metadata file), without object keys, which the reader derives from the titles of all objects. That excludes comments, the embedded model block, the stamp line itself, and label values (the model has no field for them), and objects are sorted by uniqueid. Digests: the same two forms as the JSON stamp, over the XDF view.
- This catches real edits made in TunerPro (values, addresses, scaling, axes, added or deleted tables). It ignores TunerPro reformatting, attribute order and table order.
- Where it is stored:
  - In the metadata file: `xdf.digests` (both forms, for the whole view). Implemented.
  - Best effort, in the XDF itself (not written yet; the reader already leaves it out of the view): one line `xdfkit-stamp: sha256:<digest>` in the `XDFHEADER` description, a field TunerPro shows and preserves. That line is excluded from the XDF view. TunerPro keeps it through a save (confirmed), writing that line break as `&#010;` and others as `&#013;&#010;`, so readers accept both.
- A tuner subset (`model.md`, `provenance.subset`) adds the line `xdfkit-subset: tuner sha256:<table>` to the same description. The reader takes it out of the project name and into `provenance.subset`, so a tuner XDF read without its metadata file, or with another XDF's, still reads as a subset.

## Metadata file

Implemented in `xdf/meta.go` (`xdf.Meta`, `xdf.Read`).

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
- `objects`: per object, keyed by its XDF `uniqueid`, the merge patch from its view to the model object, left out when empty. Typical entries: the key, the full id (the XDF title has only its first word), the comment (merged into the XDF description), category ids, `inverse`, `marked`, `value.description`, the display base, axis `mirror`, `header` and `signature`, the storage of "subtract" axes written as labels, and precision the writer limited to six digits. Keyed by uniqueid rather than title because titles repeat; TunerPro keeps uniqueids through a save (observed on 8D0907551M).
- `xdf`: the digests of the view (see XDF stamp).
- Its own `stamp`.
- Reconstruction (`xdf.Read`): model = XDF view merged with metadata, by uniqueid.
  - Digest matches `xdf.digests`: the merge is exact, so XDF plus metadata reproduces the model and JSON to XDF to JSON is lossless (tested on every archived pack, with and without image).
  - Digest differs (edited in TunerPro): warning. The project takes the `model` patch over the XDF's header, and XDF categories missing from the metadata are added. Each object takes the XDF's values, and keeps the metadata's for what XDF can't express (key, `inverse`, `marked`, `value.description`, display settings, axis `mirror`, `header`, `signature`, axis storage the XDF can't show) and where the metadata's value lowers to what the XDF holds (id, description and comment when the title and description are unchanged; category, shape, units, conversion, precision). Unedited objects therefore come out as before. A warning names each object whose metadata fields were not applied; it can't name which XDF values changed, because the metadata file doesn't hold the original view.
  - Objects in the metadata but missing from the XDF are dropped with a warning; objects new in the XDF (or with a repeated uniqueid) have no metadata, get a key from their title, and are reported.
  - Metadata stamp doesn't match: someone hand edited the metadata file. Warn and use it anyway.
  - No metadata file: the plain XDF view, with provenance format `xdf`.
- The XDF view: title as id, description as description, `CATEGORYMEM` as categories, cells from `EMBEDDEDDATA` (type flags: signed, little endian, float), the equation as factor and offset (any expression linear in X, or factor/X + offset for KP's reciprocal; anything else reads as X with a warning), `decimalpl` (default: `DEFAULTS sigdigits`) as precision, output type 3 as base 16 and the others as base 10. Table axes with a location are image axes; label axes are ordinal, and dropped when they have at most one label. Elements other than tables and constants (`XDFFLAG`, `XDFPATCH`, `XDFFUNCTION`) are ignored with a warning. Input that isn't valid UTF-8 is read as Windows-1252.

## Relation to the embedded model block

- The metadata file is the durable, primary copy of the residue. The comment block in `xdf-embedding.md` doesn't survive a TunerPro save (confirmed).
- The embedded block is optional (`--embed`, off by default) and carries the same payload as the metadata file (residue plus digests, XML encoded), not a second full model. Keeping one source of truth avoids the two copies disagreeing.
