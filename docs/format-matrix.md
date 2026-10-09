# Map definition formats: field support

Written 2026-10-07. KP v1/v2 as parsed by ecuxplot's mapdump (evidence from raw dumps of ecuxplot's `8D0907551G.kp`, v1, 4170 maps, and `8D0907551M.kp`, v2, 471 maps); A2L per ASAM MCD-2MC; DAMOS `.dam` (undocumented, mostly unconfirmed); XDF as TunerPro XML (mapdump writes 1.50, TunerPro 5.00 saves 1.80), checked against TunerPro's version history up to 5.00.10305 (2026-01); WinOLS script per the "Importing with scripts" help topic (map creation only, write only); the model is xdfkit's canonical model in JSON or YAML (`model.md`): "schema 1" is what `model/schema.json` holds today, "target" is the design, not implemented beyond schema 1.

Legend: `Y` supported, `P` partial or lossy, `N` not supported, `?` unconfirmed (samples needed), `-` not applicable.

| Group | Field | KP v1 | KP v2 | A2L | DAMOS | XDF | WinOLS script | Model (schema 1) | Model (target) |
|---|---|---|---|---|---|---|---|---|---|
| Identity | Short identifier | Y: id, e.g. KFMIOP | Y: id | Y: object name | Y | Y: title | Y: IdName | Y: id | Y |
| Identity | Long description | Y: name | Y: name | Y: LongIdentifier | Y | Y: description | Y: Name | Y: description | Y |
| Identity | Free-text comment | N | Y: comment; in 8D0907551M it holds hex signature bytes | P: ANNOTATION | ? | P: only by appending to description | Y: Kommentar | Y: comment | Y |
| Identity | Categories | Y: one folder per map (321 folders in 8D0907551G) | Y: one folder per map | Y: FUNCTION/GROUP, nested, many-to-many | P: function assignment | P: a path of up to 3 levels per object (CATEGORYMEM index 0 to 2 is category A, B, C), not many-to-many; up to 2048 categories, but builds before 5.00.9813 (2023) misload numbers above 255 (323 load correctly in 5.00.10305) | P: FolderName, one folder per map (WinOLS 2.08+) | P: flat categories, several per object, no nesting | Y |
| Identity | Project / ECU metadata | P: name and version parsed; most header fields unknown | P: same | Y: PROJECT, HEADER, MODULE, MOD_PAR (EPK, CPU) | P | P: title, description, author | N | P: name and version | Y |
| Addressing | Address is a file offset | Y | Y | N: ECU CPU address; needs a segment-to-file mapping | N: ECU address | Y: plus header baseoffset | Y: StartAddr / DataAddr (hexdump address, decimal) | P: file offset only; no CPU address or segment | Y: file offset plus original CPU address and segment |
| Addressing | Memory segments | N | N | Y: MEMORY_SEGMENT | ? | P: REGION (size only) | N | N | Y |
| Addressing | Checksum definition | N: separate WinOLS checksum modules | N | P: vendor-specific IF_DATA | N | Y: CHECKSUM (plugin) | N | N | Y |
| Addressing | Search signature (find map in other SW) | Y: per map and per axis | Y | N | N | N | P: search commands place maps at import; not stored with the map | N: not kept (the axis `signature` is the marker byte) | Y |
| Data layout | Single value / 1D / 2D | Y | Y | Y: VALUE / CURVE / MAP | Y: Festwert / Kennlinie / Kennfeld | Y: CONSTANT / TABLE | Y: Typ eEinzel / eEindim / eZweidim | Y: shape value / 1d / 2d | Y |
| Data layout | 3D and higher | N | N | Y: CUBOID, CUBE_4, CUBE_5 | N | N | N | N | Y |
| Data layout | Value block (array, no axes) | P: 2D with ordinal 1,2,3 axes | P: same | Y: VAL_BLK | ? | Y: table with label axes | P: 2D with no-axis data source | P: 2d with ordinal axes | Y |
| Data layout | Strings | N | N | Y: ASCII | ? | P: outputtype 4 | N | N | Y |
| Data layout | Integer 8/16/32, signed | Y | Y | Y: also 64-bit | Y | Y | Y: DataOrg + bVorzeichen | Y: bits 8/16/32, signed | Y |
| Data layout | Float | Y: 32-bit only | Y: 32-bit only | Y: 16/32/64-bit | ? | Y: float flag, 32-bit; 64-bit unconfirmed. Added in 5.00.8008 (2012) with no equations on float data (only `X`); whether that limit still holds is unconfirmed | Y: eFloatLoHi / eFloatHiLo, 32-bit | Y: 32-bit | Y |
| Data layout | Endianness per object | Y: HiLo/LoHi in type enum | Y | Y: BYTE_ORDER, module default | ?: likely global | Y: mmedtypeflags | Y: DataOrg | Y: endian | Y |
| Data layout | Row vs column major | Y: '2d Inverse' (252 of 4170 maps in 8D0907551G) | Y: '2d Inverse' (99 of 471 in 8D0907551M) | Y: RECORD_LAYOUT | ? | Y: major stride / flags | Y: Typ eZweiInv | P: `inverse` flag (KP) | Y |
| Data layout | Gaps, alignment, interleaved records | N | N | Y: RECORD_LAYOUT positions, ALIGNMENT_* | ? | P: stride only | P: SkipBytes on axes only | N | Y |
| Data layout | Bit mask / sub-byte fields | ?: not decoded; may be in an unknown block | ? | Y: BIT_MASK | ? | P: XDFFLAG, single bit | N | N | Y |
| Conversion | Linear factor + offset | Y | Y | Y: LINEAR / RAT_FUNC | Y | Y: equation | Y: Faktor / Offset (help example uses a decimal comma; locale to test) | Y: factor, offset | Y |
| Conversion | Reciprocal (f / x) | Y: flag | Y: flag | Y: RAT_FUNC | ? | Y: equation | Y: bKehrwert | Y: reciprocal flag | Y |
| Conversion | General rational (6 coefficients) | N | N | Y: RAT_FUNC | ? | Y: equation | N | N | Y |
| Conversion | Arbitrary formula | N | N | Y: FORMULA (+ inverse) | N | Y: equation; can reference other objects | N | N | Y |
| Conversion | Enum / lookup tables | N | N | Y: COMPU_TAB, COMPU_VTAB, VTAB_RANGE | ? | N | N | N | Y |
| Conversion | Units | Y | Y | Y: COMPU_METHOD unit / PHYS_UNIT | Y | Y | Y: Einheit | Y | Y |
| Conversion | Display precision | Y | Y | Y: FORMAT | ? | Y: decimalpl | Y: Nachkommastellen | Y | Y |
| Conversion | Display base dec / hex / binary | Y: base field | Y: base 10/16/2; binary on codewords, e.g. CWMSRCAN | N | ? | P: dec/hex only | P: Radix 10 / 16; binary not documented | Y: view base | Y |
| Conversion | Min / max limits | ?: range is 0-255 on almost every map; `rangeUnk00` holds two more pairs (`kp-format.md`) | ?: same; other pairs on hand-edited maps; limits or display range unconfirmed | Y: limits + EXTENDED_LIMITS | ? | Y: min / max | N: no property | N | Y |
| Conversion | Difference / percent view vs original | Y: flags | Y: flags D, P | N | N | N | Y: bDelta / bProzent | Y: view difference / percent | Y |
| Conversion | Unidentified per-map doubles | P: `viewScale` and `viewOffset` are probably display scaling, not conversion; `storedRowsUnk06` and `storedRowsUnk1B` (300.0, 1.0) unknown (`kp-format.md`) | P: same | - | - | - | - | N | N: not in the model until identified; KP output takes them from a template KP or defaults |
| Axes | Axis read from image | Y: datasource EEPROM | Y | Y: STD_AXIS / COM_AXIS | Y | Y: EMBEDDEDDATA | Y: DataSrc eRom | Y: source image | Y |
| Axes | Per-axis type, sign, units, conversion | Y | Y | Y | Y | Y | Y: StuetzX/Y.* | Y | Y |
| Axes | Fixed axis values (not in image) | Y: '1,2,3' ordinal or 'Free editable'; axis block sized 4 bytes per point | Y: '1,2,3' ordinal; free-editable storage possibly the axis `signedUnk00` list (zeros in the only sample) | Y: FIX_AXIS_PAR / _DIST / _LIST | ? | Y: static LABELs | P: DataSrc eUserdef, but no property for the values | P: source editable; the values aren't stored | Y |
| Axes | Shared axis object | N: address repeated per map | N | Y: COM_AXIS -> AXIS_PTS | ? | Y: axis linked to another table (`embedinfo type="3" linkobjid`, the table's uniqueid; TunerPro 5) | N | N: address repeated per axis | Y |
| Axes | Axis point count read from image | ? | ? | Y: NO_AXIS_PTS in record layout | ? | N | ?: DataHeader marks header bytes; whether the count is read is undocumented | N: `header` counts header bytes; the count isn't read | Y |
| Axes | Axis stored as differences | Y: 'EEPROM, subtract' = gaps between breakpoints, counted down from 256 (`kp-format.md`); 273 axes in 8D0907558E; 'add' and 'backwards' still unknown | Y: same datasource field; ME7 packs have mislabelled absolute axes | Y: DEPOSIT DIFFERENCE | ? | P: static LABELs computed from the bin; or live, through a linked helper table with `CELL()` equations (`kp-format.md`; XML unconfirmed) | Y: DataSrc eRomAdd / eRomSub; also eRomBackwards | Y: stored absolute / add / subtract / backwards (add and backwards semantics unknown) | Y |
| Axes | Rescale / curve axes | N | N | Y: RES_AXIS, CURVE_AXIS | N | P: axis "Normalized" through a function (XDFFUNCTION, Ford-style normalizer); how closely it matches CURVE_AXIS is unconfirmed | N | N | Y |
| Tool-specific | Patches | N | N | N | N | Y: XDFPATCH | N | N | Y |
| Tool-specific | Measurements (logging channels) | N | N | Y: MEASUREMENT; useful for ecuxplot | N | N | N | N | Y |

## Totals

| Format | Y | P | N | ? |
|---|---|---|---|---|
| KP v1 | 20 | 2 | 13 | 4 |
| KP v2 | 21 | 2 | 12 | 4 |
| A2L | 31 | 2 | 5 | 0 |
| DAMOS | 8 | 2 | 9 | 19 |
| XDF | 22 | 10 | 6 | 0 |
| WinOLS script | 17 | 6 | 14 | 1 |
| Model (schema 1) | 16 | 6 | 17 | 0 |
| Model (target) | 38 | 0 | 1 | 0 |
