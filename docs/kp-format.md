# KP format

WinOLS map pack (`*.kp`), read and written by `kp/`. Integers are little-endian. A string is an int32 byte count, that many ISO-8859-15 bytes, and a NUL. A zero count has no NUL.

The Go structs are the record layouts. `kp/codec.go` walks fields in declaration order (tag grammar at the top of that file). JSON names are those fields with a lower-case first letter (`kp-defaults.json`, `model.md`). File, Project and Folder always use that walk. Headers 503 and 597 use a hand walk for Map and Axis (`kp/ols5.go`); their `kp` tags are not the source of truth for those headers.

## Versions

The int32 at offset 0x10 is a decimal header id. Headers that share a row share a record walk, named by hundred: 100 (113 and 116), 200+300 (292 and 329), and 500 (503 and 597).

| Header | Walk | Value at 0x14 | Fixed header ends | Maps |
| --- | --- | --- | --- | --- |
| 113, 116 | 100 | 0 | 0x5c | inline |
| 292, 329 | 200+300 | file length | 0x60 | zip entry `intern` |
| 503, 597 | 500 | file length | 0x60 | zip entry `intern` |

329 is accepted; archived packs use 292. WinOLS 2.24 writes header 292 and version `OLS 5.0 (Windows)`. 503 and 597 are read-only (`Encode` errors until a save oracle exists). Size differences and OLS5 string modes live in `kp/ols5.go`.

Every archived pack ends with `33 88 72 98`. `EndOffset` is that marker's offset (file length minus 4). KP-only residue defaults in `kp-defaults.json` are pooled by hundred (`"100"`, `"200+300"`, `"500"`); `"default"` holds values shared by at least two pools.

## Layout

```mermaid
flowchart TD
  subgraph file [File]
    A["signature, header id, length"] --> B["fixed header through 0x5c or 0x60"]
    B --> C["filename, version, header body"]
    C --> D["EndOffset and end tag"]
    D --> E[Project]
  end
  E --> F[Maps]
  F --> G["MapsTag, folders, trailing bytes"]
  F --> H["113 and 116: count, then records"]
  F --> I["292, 329, 503, 597: zip length, then the zip"]
  I --> J["intern: flag 0, count, records"]
```

`FixedHeader` (0x18 through the fixed end) and `HeaderBody` (version string through `EndOffset`), plus `Project.Trailing`, are kept raw. After `EndOffset`: end tag, project name/version (and trailers on 503/597), then `Build`. Field sizes and tag constants: `kp/` structs.

Maps, then `MapsTag`, then folders:

- 113/116: int32 count, then map records
- 292/329/503/597: int32 zip length, then the zip; folders follow. Inflated `intern` is flag 0, count, records

Each folder: id, `BuiltIn` (9000 My maps, 9001 Hexdump, else 0), name, flags, style, pad. This reader keeps the table as stored.

## Round trip

`Parse` and `Encode` share one walk. Encode writes the length at 0x14 and `EndOffset` for 292/329. Encode of 503/597 is not supported.

- 113/116: parse then encode is byte-identical (including JSON in between)
- 292/329: original zip reused while the map block is unchanged; a changed block is re-deflated with Go `compress/flate` level 9. Inflated block and bytes outside the zip match aside from the two lengths

Decoding fails when a bool is not 0 or 1, a string has no NUL, a free-editable list length is not a multiple of 4, or bytes remain after the maps on 292/329.

## Zip on 292+

One local entry, name `intern`, method 8. Archived streams match zlib 1.2.x raw deflate at level 9. Go's deflate differs, so a rewritten file must update the zip length, the length at 0x14, and `EndOffset`.

## Map

Order is the `kp.Map` struct. Writers need:

- `Marked` → model `vislevel` 1 when set, else 0
- `Organization`, `Type` / `Width`, `Base` (10/16/2): [Enums](#enums)
- `ID` (short name), `Name` (long description), optional `Comment` (v2)
- `Range`: three low/high pairs for 8/16/32-bit raw spans. Without a template, the pair for the map's width is the model's min/max converted to raw; the other pairs stay at defaults. The model omits min/max when the KP pair is that default.
- Flags: `Reciprocal`, `Signed`, `Difference`, `Percent` (model `view`)
- `Cols`, `Rows`, `Precision`, `Value` (description, units, factor, offset)
- `Start`, `End` (`End` = start + byte length; MP01 in `autocorrect.md`), `ImageSize`, `Addr2`
- Axes `X`, `Y`: [Axis](#axis)
- `StoredCols` / `StoredRows` follow `Cols`/`Rows`, swapped on organisation 5 (2D inverse)
- Tail after Y (view mode, cursor, etc.) is residue; see `kp.Map`

## Axis

Both records are stored on every map. WinOLS uses X for 1D, X and Y for 2D, neither for a single value.

| Field | Notes |
| --- | --- |
| `Value` | description, units, factor, offset |
| `DataSource` | [Enums](#enums). Address matters for image sources |
| `Addr`, `Type`, `Width`, `Base` | `Width` is element size in bytes |
| `Mirror` | image stays stored order; WinOLS shows high end first |
| `Reciprocal`, `Precision`, `Signed` | |
| `Values` | free-editable list: length is 4× point count; points packed in element type, zero-padded |
| `DataHeader` | bytes before axis data treated as header (0/1/2/4); model `header` |
| `SignatureByte` | `-1` means none; model `signature`, omitted for `-1` |

On 292/329, `Values` is empty except on free-editable axes. Free editable (datasource 4) clears the axis address and sets factor to 1.

## Enums

| Field | Values |
| --- | --- |
| Organisation | 2 single, 3 1D, 4 2D, 5 2D inverse |
| Value type | 1 8-bit, 2 16-bit HiLo, 3 16-bit LoHi, 4 32-bit HiLo, 5 32-bit LoHi, 6 float HiLo, 7 float LoHi |
| Data source | 0 ordinal, 1 EEPROM, 2 EEPROM add, 3 EEPROM subtract, 4 free editable, 5 EEPROM backwards |
| Display base | 10, 16, 2 (omitted → 10; base 0 crashes WinOLS) |

## EEPROM storage

Displayed point i, after conversion:

- **add** (datasource 2): `raw[0] + … + raw[i]` (no wrap)
- **subtract** (datasource 3): `2^bits - (raw[i] + … + raw[n-1])` (no wrap); last raw is the gap to the top of the range
- **backwards** (datasource 5): `raw[n - 1 - i]`

The model keeps `axis.stored` as `absolute`, `add`, `subtract`, or `backwards`. Mislabelled packs are fixed by DS01/DS02 (`autocorrect.md`). XDF live helpers for add/subtract: `stamp-and-metadata.md`.

## WinOLS 2.24 exports

Compared with an imported pack: header 292; `End` rewritten to start + byte length where stale; `Cursor` / `Marked` / `ViewScale` may change on opened maps; `intern` matches zlib level 9.
