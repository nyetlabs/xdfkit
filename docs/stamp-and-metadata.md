# Edit stamp and XDF metadata sidecar

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
  - YAML will be encoded from the same tree and keep the int/float distinction.

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

- An XDF has no JSON of its own, so its digest covers the XDF view: the JSON that xdfkit's XDF reader builds from the XDF elements alone. That excludes comments, the embedded model block and the stamp line itself, and objects are sorted by id. Digests: the same two forms as the JSON stamp, over the XDF view.
- This catches real edits made in TunerPro (values, addresses, scaling, axes, added or deleted tables). It ignores TunerPro reformatting, attribute order and table order.
- Where it is stored:
  - In the metadata file: `xdf.digest`, plus one digest per object (`xdf.objects[id]`), so edits can be pinned to specific tables.
  - Best effort, in the XDF itself: one line `xdfkit-stamp: sha256:<digest>` in the `XDFHEADER` description, a field TunerPro shows and preserves. That line is excluded from the XDF view. Confirmed 2026-10-07: TunerPro shows the description as two lines and keeps the stamp line through a save (writing the line break as `&#010;`; it writes other description line breaks as `&#013;&#010;`, so readers accept both).

## Metadata sidecar

- Written next to every XDF: `name.xdf` and `name.meta.json` (see `naming.md`).
- Contents: everything the XDF lowering dropped or approximated. It is the residue, not the full model:
  - Per object, keyed by the same id as the XDF table title: the fields XDF can't express (see `format-matrix.md`, the XDF column), the loss-report entries for that object, and the per-source raw block (for KP, the unidentified fields).
  - Shared objects (axes, conversions, record layouts) and project-level data (source file name and SHA-256, KP header fields, folders).
  - `xdf`: the digest of the XDF view it was generated with, plus per-object digests.
  - Its own `stamp`.
- Reconstruction: model = XDF view merged with metadata.
  - XDF digest matches `xdf.digest`: the merge is exact. XDF plus metadata reproduces the full model, so XDF to JSON to XDF is lossless.
  - XDF digest differs (edited in TunerPro): merge per object. Unchanged objects get all their metadata. Changed objects take the XDF's values and keep only the metadata fields XDF can't express, with a warning that names them. Objects missing from the XDF are dropped with a warning; new XDF objects have no metadata.
  - Metadata stamp doesn't match: someone hand edited the sidecar. Warn and use it anyway.
  - No metadata file: plain lossy XDF read.

## Relation to the embedded model block

- The sidecar is the durable, primary copy of the residue. The comment block in `xdf-embedding.md` doesn't survive a TunerPro save (confirmed).
- Decided 2026-10-07: the embedded block is optional (`--embed`, off by default) and carries the same payload as the sidecar (residue plus digests, XML encoded), not a second full model. Keeping one source of truth avoids the two copies disagreeing.

## Decided

- Sidecar file name: `name.meta.json` next to `name.xdf` (2026-10-07; see `naming.md`).
