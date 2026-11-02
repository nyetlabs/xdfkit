# Map definition formats: field support

KP (ecuxplot mapdump headers), A2L, DAMOS, XDF 1.80, and the canonical model (`model.md`). Schema 1 is `model/schema.json` today; Target is the design beyond that.

Legend: `Y` supported, `P` partial or lossy, `N` not supported, `?` unconfirmed, `-` not applicable.

| Group | Field | KP 113/116 | KP 292/329 | A2L | DAMOS | XDF | Model | Target |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| Identity | Short identifier | Y | Y | Y | Y | Y | Y | Y |
| Identity | Long description | Y | Y | Y | Y | Y | Y | Y |
| Identity | Comment | N | Y | P | ? | P | Y | Y |
| Identity | Categories | Y | Y | Y | P | P | P | Y |
| Identity | Project / ECU metadata | P | P | Y | P | P | P | Y |
| Addressing | File offset | Y | Y | N | N | Y | P | Y |
| Addressing | Memory segments | N | N | Y | ? | P | N | Y |
| Addressing | Checksum definition | N | N | P | N | Y | N | Y |
| Addressing | Search signature | Y | Y | N | N | N | N | Y |
| Data layout | Single value / 1D / 2D | Y | Y | Y | Y | Y | Y | Y |
| Data layout | 3D and higher | N | N | Y | N | N | N | Y |
| Data layout | Value block, no axes | P | P | Y | Y | Y | P | Y |
| Data layout | Strings | N | N | Y | ? | P | N | Y |
| Data layout | Integer 8/16/32, signed | Y | Y | Y | Y | Y | Y | Y |
| Data layout | Float | Y | Y | Y | ? | Y | Y | Y |
| Data layout | Endianness | Y | Y | Y | ? | Y | Y | Y |
| Data layout | Row vs column major | Y | Y | Y | ? | Y | P | Y |
| Data layout | Gaps / alignment | N | N | Y | Y | P | N | Y |
| Data layout | Bit mask | ? | ? | Y | ? | P | N | Y |
| Conversion | Factor + offset | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Reciprocal (f / x) | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Rational, 6 coefficients | N | N | Y | ? | Y | N | Y |
| Conversion | Arbitrary formula | N | N | Y | N | Y | N | Y |
| Conversion | Enum / lookup table | N | N | Y | ? | N | N | Y |
| Conversion | Units | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Display precision | Y | Y | Y | ? | Y | Y | Y |
| Conversion | Display base | Y | Y | N | ? | P | Y | Y |
| Conversion | Min / max | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Difference / percent | Y | Y | N | N | N | Y | Y |
| Conversion | Per-map doubles | P | P | - | - | - | N | N |
| Axes | Read from image | Y | Y | Y | Y | Y | Y | Y |
| Axes | Type, sign, units, conversion | Y | Y | Y | Y | Y | Y | Y |
| Axes | Fixed values | Y | Y | Y | ? | Y | P | Y |
| Axes | Shared axis object | N | N | Y | Y | Y | N | Y |
| Axes | Point count from image | ? | ? | Y | ? | N | N | Y |
| Axes | Stored as differences | Y | Y | Y | ? | P | Y | Y |
| Axes | Rescale / curve | N | N | Y | N | P | N | Y |
| Tool-specific | Map mark / visibility | N | Y | N | ? | Y | Y | Y |
| Tool-specific | Patches | N | N | N | N | Y | N | Y |
| Tool-specific | Measurements | N | N | Y | N | N | N | Y |

Names and limits for the rows above:

- Short identifier. KP `id`. A2L object name. XDF `title`. Model `id`.
- Long description. KP `name`. A2L `LongIdentifier`. XDF `description`. Model `description`.
- Comment. Header 292/329 `comment`. A2L `ANNOTATION`. XDF only by appending to `description`.
- Categories. KP: one folder per map. A2L: `FUNCTION` / `GROUP`, nested, many-to-many. DAMOS: function assignment. XDF: one path of up to 3 levels (`CATEGORYMEM`), not many-to-many. Model: flat, several per object.
- Project / ECU metadata. KP: name and version. A2L: `PROJECT` / `HEADER` / `MODULE` / `MOD_PAR`. XDF: title, description, author. Model: name and version.
- File offset. A2L/DAMOS store an ECU address (need a segment map). XDF adds `baseoffset`. Model is a file offset only; target adds CPU address and segment.
- Memory segments. A2L `MEMORY_SEGMENT`. XDF `REGION` is a size only.
- Checksum definition. KP: separate WinOLS modules. A2L: vendor `IF_DATA`. XDF `CHECKSUM` plugin.
- Search signature. KP per map and axis; not in the model (axis `signature` is a different marker byte). Target keeps it.
- Single value / 1D / 2D. A2L `VALUE` / `CURVE` / `MAP`. DAMOS Festwert / Kennlinie / Kennfeld. XDF `CONSTANT` / `TABLE`. Model `value` / `1d` / `2d`.
- 3D and higher. A2L `CUBOID`, `CUBE_4`, `CUBE_5`.
- Value block. KP/model: 2D map with ordinal axes. A2L `VAL_BLK`. DAMOS type 13. XDF: table with label axes.
- Strings. A2L `ASCII`. XDF `outputtype` 4.
- Integer. A2L also has 64-bit. Model `bits` 8/16/32 and `signed`.
- Float. KP and model are 32-bit. A2L 16/32/64. XDF `mmedtypeflags` bit 0x10000 is float32 only; the writer rejects non-32-bit float and non-identity equations on float tables.
- Endianness. KP type enum. A2L `BYTE_ORDER`. XDF `mmedtypeflags` (absent wider-cell flags follow `DEFAULTS lsbfirst`). Model `endian`.
- Row vs column major. KP `2d Inverse`. A2L `RECORD_LAYOUT`. XDF stride/flags. Model `inverse`.
- Gaps / alignment. A2L `RECORD_LAYOUT` / `ALIGNMENT_*`. DAMOS even-address pad (`model.md`). XDF stride only; model does not store the gap.
- Bit mask. KP not decoded. A2L `BIT_MASK`. XDF `XDFFLAG`.
- Factor + offset. A2L `LINEAR` / `RAT_FUNC`. XDF equation. Reciprocal / rational: `model.md`, Conversions.
- Arbitrary formula. A2L `FORMULA` (+ inverse). XDF equation (can reference other objects).
- Enum / lookup table. A2L `COMPU_TAB`, `COMPU_VTAB`, `VTAB_RANGE`.
- Units. A2L `COMPU_METHOD` / `PHYS_UNIT`. Display precision: A2L `FORMAT`, XDF `decimalpl`.
- Display base. KP base field. XDF decimal/hex only. Model `view.base`.
- Min / max. KP `range` per width (`kp-format.md`). A2L limits / `EXTENDED_LIMITS`. DAMOS `/SPW`/`/SPX`/`/SPY`. Model `min`/`max` (`model.md`).
- Difference / percent. KP map-window flags. Model view flags.
- Per-map doubles. KP view state (`viewScale`, …); not in the model — template or defaults on KP output.
- Read from image. KP EEPROM datasource. A2L `STD_AXIS` / `COM_AXIS`. XDF `EMBEDDEDDATA`. Model source `image`.
- Fixed values. KP ordinal or free editable (`kp-format.md`). A2L `FIX_AXIS_*`. XDF `LABEL`s. Model source `editable` with `values`.
- Shared axis object. KP/model repeat the address. A2L `COM_AXIS` → `AXIS_PTS`. DAMOS type 7/8 copies a type-6 table (`model.md`). XDF `embedinfo type="3"`.
- Point count from image. A2L `NO_AXIS_PTS`. Model `header` is a byte count; point count not read.
- Stored as differences. KP add/subtract/backwards formulas (`kp-format.md`); XDF live helpers (`stamp-and-metadata.md`). Model `absolute` / `add` / `subtract` / `backwards`. A2L `DEPOSIT DIFFERENCE`.
- Rescale / curve. A2L `RES_AXIS` / `CURVE_AXIS`. XDF "Normalized" via `XDFFUNCTION` (relation to `CURVE_AXIS` unconfirmed).
- Map mark / visibility. KP `Marked` ↔ model `vislevel` 1; unmarked 0 → XDF level 2 (`model.md`). `IDState` on 292/329 is a different unconfirmed field. Metadata carries `vislevel` when model differs from the XDF view.
- Patches. XDF `XDFPATCH`. Measurements. A2L `MEASUREMENT`.

## Totals

| Format | Y | P | N | ? |
|---|---|---|---|---|
| KP 113/116 | 21 | 3 | 14 | 2 |
| KP 292/329 | 23 | 3 | 12 | 2 |
| A2L | 31 | 2 | 6 | 0 |
| DAMOS | 13 | 2 | 9 | 15 |
| XDF | 23 | 10 | 6 | 0 |
| Model (schema 1) | 18 | 6 | 16 | 0 |
| Model (target) | 39 | 0 | 1 | 0 |
