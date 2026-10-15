# Map definition formats: field support

KP v1/v2 as parsed by ecuxplot's mapdump (evidence from raw dumps of ecuxplot's `8D0907551G.kp`, v1, 4170 maps, and `8D0907551M.kp`, v2, 471 maps); A2L per ASAM MCD-2MC; DAMOS `.dam` (undocumented, mostly unconfirmed); XDF as TunerPro XML (mapdump writes 1.50, TunerPro 5.00 saves 1.80), checked against TunerPro's version history up to 5.00.10305 (2026-01); the model is xdfkit's canonical model in JSON or YAML (`model.md`): "schema 1" is what `model/schema.json` holds today, "target" is the design, not implemented beyond schema 1.

Legend: `Y` supported, `P` partial or lossy, `N` not supported, `?` unconfirmed (samples needed), `-` not applicable. Model is schema 1. Target is the design.

| Group | Field | KP v1 | KP v2 | A2L | DAMOS | XDF | Model | Target |
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
| Data layout | Value block, no axes | P | P | Y | ? | Y | P | Y |
| Data layout | Strings | N | N | Y | ? | P | N | Y |
| Data layout | Integer 8/16/32, signed | Y | Y | Y | Y | Y | Y | Y |
| Data layout | Float | Y | Y | Y | ? | Y | Y | Y |
| Data layout | Endianness | Y | Y | Y | ? | Y | Y | Y |
| Data layout | Row vs column major | Y | Y | Y | ? | Y | P | Y |
| Data layout | Gaps / alignment | N | N | Y | ? | P | N | Y |
| Data layout | Bit mask | ? | ? | Y | ? | P | N | Y |
| Conversion | Factor + offset | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Reciprocal (f / x) | Y | Y | Y | ? | Y | Y | Y |
| Conversion | Rational, 6 coefficients | N | N | Y | ? | Y | N | Y |
| Conversion | Arbitrary formula | N | N | Y | N | Y | N | Y |
| Conversion | Enum / lookup table | N | N | Y | ? | N | N | Y |
| Conversion | Units | Y | Y | Y | Y | Y | Y | Y |
| Conversion | Display precision | Y | Y | Y | ? | Y | Y | Y |
| Conversion | Display base | Y | Y | N | ? | P | Y | Y |
| Conversion | Min / max | Y | Y | Y | ? | Y | Y | Y |
| Conversion | Difference / percent | Y | Y | N | N | N | Y | Y |
| Conversion | Per-map doubles | P | P | - | - | - | N | N |
| Axes | Read from image | Y | Y | Y | Y | Y | Y | Y |
| Axes | Type, sign, units, conversion | Y | Y | Y | Y | Y | Y | Y |
| Axes | Fixed values | Y | Y | Y | ? | Y | P | Y |
| Axes | Shared axis object | N | N | Y | ? | Y | N | Y |
| Axes | Point count from image | ? | ? | Y | ? | N | N | Y |
| Axes | Stored as differences | Y | Y | Y | ? | P | Y | Y |
| Axes | Rescale / curve | N | N | Y | N | P | N | Y |
| Tool-specific | Map mark / visibility | N | Y | N | ? | P | Y | Y |
| Tool-specific | Patches | N | N | N | N | Y | N | Y |
| Tool-specific | Measurements | N | N | Y | N | N | N | Y |

Names and limits for the rows above:

- Short identifier. KP `id`. A2L object name. XDF `title`. Model `id`.
- Long description. KP `name`. A2L `LongIdentifier`. XDF `description`. Model `description`.
- Comment. KP v2 `comment`. In 8D0907551M that string holds hex signature bytes. A2L `ANNOTATION`. XDF only by appending to `description`.
- Categories. KP: one folder per map (321 in 8D0907551G). A2L: `FUNCTION` / `GROUP`, nested, many-to-many. DAMOS: a function assignment. XDF: one path of up to 3 levels (`CATEGORYMEM` 0 to 2), not many-to-many; up to 2048 categories; builds before 5.00.9813 (2023) misload numbers above 255 (323 load in 5.00.10305). Model: flat, several per object, no nesting.
- Project / ECU metadata. KP: name and version; most header fields are unknown. A2L: `PROJECT`, `HEADER`, `MODULE`, `MOD_PAR` (EPK, CPU). XDF: title, description, author. Model: name and version.
- File offset. A2L and DAMOS store an ECU address and need a segment map. XDF adds the header `baseoffset`. Model is a file offset only. Target adds the original CPU address and segment.
- Memory segments. A2L `MEMORY_SEGMENT`. XDF `REGION` is a size only.
- Checksum definition. KP uses separate WinOLS checksum modules. A2L is vendor `IF_DATA`. XDF `CHECKSUM` is a plugin.
- Search signature. KP: per map and per axis. Model does not keep it. The axis `signature` is a marker byte, not this. Target keeps it.
- Single value / 1D / 2D. A2L `VALUE` / `CURVE` / `MAP`. DAMOS Festwert / Kennlinie / Kennfeld. XDF `CONSTANT` / `TABLE`. Model `value` / `1d` / `2d`.
- 3D and higher. A2L `CUBOID`, `CUBE_4`, `CUBE_5`.
- Value block. KP and the model: a 2D map with ordinal axes. A2L `VAL_BLK`. XDF: a table with label axes.
- Strings. A2L `ASCII`. XDF `outputtype` 4.
- Integer. A2L also has 64-bit. Model `bits` 8/16/32 and `signed`.
- Float. KP and the model are 32-bit (`eFloatLoHi` / `eFloatHiLo`). A2L is 16/32/64-bit. XDF has a 32-bit float flag, added in 5.00.8008 (2012). 64-bit is unconfirmed. That build allowed no equation on float data except `X`; whether the limit remains is unconfirmed.
- Endianness. KP HiLo/LoHi is the type enum. A2L `BYTE_ORDER`, with a module default. DAMOS may be global. XDF `mmedtypeflags`. Model `endian`.
- Row vs column major. KP `2d Inverse`: 252 of 4170 maps in 8D0907551G, 99 of 471 in 8D0907551M. A2L `RECORD_LAYOUT`. XDF major stride and flags. Model `inverse` from KP.
- Gaps / alignment. A2L `RECORD_LAYOUT` positions and `ALIGNMENT_*`. XDF stride only.
- Bit mask. KP is not decoded; it may sit in an unknown block. A2L `BIT_MASK`. XDF `XDFFLAG`, one bit.
- Factor + offset. A2L `LINEAR` / `RAT_FUNC`. XDF equation.
- Reciprocal. KP flag. A2L `RAT_FUNC`. XDF equation. Model flag.
- Rational. A2L `RAT_FUNC`. XDF equation.
- Arbitrary formula. A2L `FORMULA`, with an inverse. XDF equation, which can reference other objects.
- Enum / lookup table. A2L `COMPU_TAB`, `COMPU_VTAB`, `VTAB_RANGE`.
- Units. A2L `COMPU_METHOD` unit / `PHYS_UNIT`.
- Display precision. A2L `FORMAT`. XDF `decimalpl`.
- Display base. KP base field; v2 uses binary on codewords such as CWMSRCAN. XDF decimal and hex only. Model view base.
- Min / max. KP `range`: one low/high pair per element width, stored as typed (`kp-format.md`). Raw versus displayed units is unconfirmed. A2L limits plus `EXTENDED_LIMITS`. Model `min` and `max` on `value`, omitted when the source has none (`model.md`).
- Difference / percent. KP flags. Model view flags.
- Per-map doubles. KP `viewScale`, `viewOffset`, `view2DScale`, and `view3DRotation` (often 300.0, 1.0) are view state, not a conversion. Not in the model. KP output takes them from a template or from defaults.
- Read from image. KP datasource EEPROM. A2L `STD_AXIS` / `COM_AXIS`. XDF `EMBEDDEDDATA`. Model source `image`.
- Per-axis type, sign, units, conversion.
- Fixed values. KP ordinal `1, 2, 3`, or free editable with 4 bytes per point. v2 stores those values in the axis list, in the axis type. A2L `FIX_AXIS_PAR`, `FIX_AXIS_DIST`, `FIX_AXIS_LIST`. XDF static `LABEL`s. Model source `editable`; the values are not stored.
- Shared axis object. KP repeats the address on each map. A2L `COM_AXIS` points at `AXIS_PTS`. XDF links an axis to another table (`embedinfo type="3"`, `linkobjid` the uniqueid; TunerPro 5). The model repeats the address. The XDF writer links an axis to a matching curve at that address.
- Point count from image. A2L `NO_AXIS_PTS` in the record layout. Model `header` is a byte count; the point count is not read.
- Stored as differences. KP `EEPROM, subtract` counts gaps down from 256 (`kp-format.md`). `add` and `backwards` are still unknown. ME7 packs include absolute axes labelled subtract. A2L `DEPOSIT DIFFERENCE`. XDF writes static labels from the image, or a live helper table of `CELL()` equations whose XML is unconfirmed. The model stores `absolute`, `add`, `subtract`, and `backwards`.
- Rescale / curve. A2L `RES_AXIS`, `CURVE_AXIS`. XDF axis "Normalized" through `XDFFUNCTION`. How close that is to `CURVE_AXIS` is unconfirmed.
- Map mark / visibility. KP v2 `Marked` is 0 or 1. Whether it is the map-list marker is unconfirmed. XDF `vislevel` is a tree-filter level ("Show Up To" / "Show Only"), not a mark. Attribute or element, range, and default are unconfirmed, and it is not written. Model `marked`. XDF keeps that in the metadata file.
- Patches. XDF `XDFPATCH`.
- Measurements. A2L `MEASUREMENT`, which ecuxplot can log.

## Totals

| Format | Y | P | N | ? |
|---|---|---|---|---|
| KP v1 | 21 | 3 | 14 | 2 |
| KP v2 | 23 | 3 | 12 | 2 |
| A2L | 31 | 2 | 6 | 0 |
| DAMOS | 8 | 2 | 9 | 20 |
| XDF | 22 | 11 | 6 | 0 |
| Model (schema 1) | 18 | 6 | 16 | 0 |
| Model (target) | 39 | 0 | 1 | 0 |
