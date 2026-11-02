# Canonical model

The format-neutral model every reader converts into and every writer converts out of. Its JSON form is the interchange format. Implemented in `model/` for KP both ways, for XDF (with metadata file, `stamp-and-metadata.md`), and for A2L and DAMOS read into schema 1.

## Conventions

- Schema: generated from Go types in `model/` (`json`, `doc`, `enum`, `pattern` tags) as `model/schema.json` (draft 2020-12). `make schema` rewrites it; tests fail on drift and validate every ecuxplot pack's model against it.
- Scope: schema 1 holds what the readers emit. Axes, conversions and record layouts are inlined. Shared lists and image identity are not in schema 1. It grows by optional fields; a breaking change bumps the version. `format-matrix.md` lists schema 1 next to the target.
- Addresses: hex strings `"0x13D52"` (uppercase on write, either case on read, at most 8 digits).
- Object `key` is the source id. A blank id leaves the key blank; repeated keys are allowed. Source id stays in `id`. XDF title holds the first word of `id`; the metadata file restores `key` and the full `id` (`stamp-and-metadata.md`).
- JSON is canonical stamped form (`stamp-and-metadata.md`).

## Document

- `schema`: `xdfkit-model/1`.
- `provenance`:
  - `format`: `kp`, `xdf` (XDF without metadata), `a2l`, `damos`, or `image` (maps located in a flash image; with the image's `sha256`).
  - `origin`: `damos`, `a2l`, `manual`, or `auto` (for format `image`).
  - `file`, `sha256`; `verified` when a person checked every definition against the image.
  - `reference` (optional): when an auto definition was checked against another — `origin`, `file`, `sha256`, `agree`, `except`, `unreferenced`. `Reference.Agreed` is true when every reference name outside `except` agrees.
  - `subset` (`kind` `tuner`, `table` SHA-256): set by `Model.Tuner`; XDF writer adds `xdfkit-subset:` to the header description.
- `Model.Tuner(table)` (`--tuner`, `ConvertRequest.Tuner`): keeps objects named in a `CategoryTable` plus objects their image axes point at; replaces categories; sets `provenance.subset`. Needs provenance. `Model.Categorize(table, rest)` files the same way but keeps every object (rest under `rest`). Category table is schema 2 (`categories.schema.json`): each category name maps to map names; a name is listed once.
- `project`: `name`, `version`.
- `categories`: `id`, `name` (KP folders).
- `objects`: in source order. Each has `key`, `id`, `description`, `comment`, `categories`, `shape` (`value` / `1d` / `2d`), `inverse` (KP "2d Inverse", unconfirmed), `vislevel` (TunerPro level; 1 = WinOLS map-list marker / KP `Marked`; unmarked 0 writes as XDF level 2), `address`, `rows`, `cols`, `data` (`bits`, `endian`, `signed`, `float`), `value` (description, units, precision, conversion, optional min/max), `view` (`base`, `difference`, `percent`), axes `x`/`y`, optional `alternates`.
- `min` / `max`: display units; omitted when the source has none. XDF writer emits only when both are set. KP stores a raw typed pair per width (`kp-format.md`); convert with the map conversion. Missing model pair → default raw span; KP default pair omitted from the model.
- `view.base`: 10, 16 or 2 (omitted → 10). WinOLS crashes on base 0. A defined axis with no storage is an 8-bit ordinal.
- `alternates`: other displays of the same cells (own id, description, categories, value, view; optional axis display). Address and storage stay on the object. `merge --smart` adds one when two objects share a raw layout and differ in units or conversion (`design.md`).
- Axes: `source` is `image`, `ordinal`, `editable` (KP "Free editable"), or `unknown` (written back as ordinal). Image axes also have `stored` (`absolute` / `add` / `subtract` / `backwards`; `kp-format.md`), `address`, `data`. Editable axes carry `data` and `values` (display units). Every axis has `value`, `view`, `mirror`, and optional `signature` (KP `SignatureByte`, unconfirmed).
- Names are format-neutral English; each writer maps them.
- `image` identity (part number, version, Bosch ident, SHA-256) is planned, not in schema 1.

### Conversions

- Model stores phys = factor×raw + offset (or reciprocal: phys = factor/raw + offset).
- DAMOS `/REP 0, B, C, 0` → reciprocal with factor B/C, offset 0. General `/REP` internal = (A×phys + B) / (C×phys + D): C = 0 and D ≠ 0 is linear (phys = (D×raw − B) / A when A ≠ 0); other non-reciprocal forms warn and store factor 1.
- A2L `RAT_FUNC` `COEFFS` (ASAM, internal = f(phys)): linear (a = d = e = 0) → factor f/b, offset −c/b; reciprocal (a = b = d = 0, e ≠ 0) → factor c/e, offset −f/e.

### ME7 record layout

Read by the DAMOS reader and by an A2L `RECORD_LAYOUT` whose counts are bytes. `/SPZ` (ECU address − 0x800000) is the record start (when the line's two hex fields differ, the second is that start).

- Values (type 1), shared-axis curves/maps (7, 8), fixed-axis curves/maps (4, 5), value blocks (13): values start at the record start.
- Inline axes (2 curve, 3 map, 6 axis-point table): one count per axis, then axes, then values. Every 16-bit field starts at an even address (pad byte when needed). Counts are one byte, except an inline map with both axes 16-bit: those two counts are 16-bit little-endian (row count then column count).
- `/FKX`: declared length (curve columns, map rows). `/FKY`: map columns. Inline map: row axis (`/SPX`) first, column axis (`/SPY`) second.
- Shared axis (7, 8): values at record start; last number on `/SPX` (and `/SPY`) is the `/SPZ` index of a type-6 axis-point table — copy that table's address, width, conversion, and `/FKX` length.
- A2L layouts with wider-than-byte counts: ASAM position order with `MOD_COMMON` alignment, not the one-byte rule. `S_REC_LAYOUT DAMOS` selects the one-byte layout (map's first axis is the rows). `ASCII` → 1d block of `NUMBER` bytes. `BYTE_ORDER MSB_LAST` → little-endian. DAMOS has no byte-order field; the reader stores little-endian and warns once.

## KP output

No format-specific residue in the model JSON. Understood values become model fields; the rest stay in the KP file.

- With `--template`: header and KP-only fields from the matching `key` / folder id. Display text is cleaned on the way in: each line of a description, units, category name, or project name and version loses its leading and trailing space, and a run of spaces or tabs becomes one space. A comment is trimmed only at the ends of the whole string. Same provenance file reproduces 113/116 aside from that text; 292/329 except a re-deflated zip when maps changed.
- Without a template: `model/empty.kp` plus `model/kp-defaults.json` (`make schema` regenerates). Derived from the model: widths, end address, `addr2`, stored dimensions, value-range pair (`kp-format.md`).
- An edited model field wins unless the template holds a value the model can't represent.
