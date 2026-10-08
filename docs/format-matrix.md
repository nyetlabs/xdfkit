# Map definition formats: field support

Written 2026-10-07. KP v1/v2 as parsed by ecuxplot's mapdump (evidence from raw dumps of ecuxplot's `8D0907551G.kp`, v1, 4170 maps, and `8D0907551M.kp`, v2, 471 maps); A2L per ASAM MCD-2MC; DAMOS `.dam` (undocumented, mostly unconfirmed); XDF as TunerPro XML (mapdump writes 1.50, TunerPro 5.00 saves 1.80), checked against TunerPro's version history up to 5.00.10305 (2026-01); WinOLS script per the "Importing with scripts" help topic (map creation only, write only); JSON/YAML is xdfkit's canonical model (`model.md`).

Legend: `Y` supported, `P` partial or lossy, `N` not supported, `?` unconfirmed (samples needed), `-` not applicable.

| Group | Field | KP v1 | KP v2 | A2L | DAMOS | XDF | WinOLS script | JSON/YAML |
|---|---|---|---|---|---|---|---|---|
| Identity | Short identifier | Y: id, e.g. KFMIOP | Y: id | Y: object name | Y | Y: title | Y: IdName | Y |
| Identity | Long description | Y: name | Y: name | Y: LongIdentifier | Y | Y: description | Y: Name | Y |
| Identity | Free-text comment | N | Y: comment; in 8D0907551M it holds hex signature bytes | P: ANNOTATION | ? | P: only by appending to description | Y: Kommentar | Y |
| Identity | Categories | Y: one folder per map (321 folders in 8D0907551G) | Y: one folder per map | Y: FUNCTION/GROUP, nested, many-to-many | P: function assignment | P: a path of up to 3 levels per object (CATEGORYMEM index 0 to 2 is category A, B, C), not many-to-many; up to 2048 categories, but builds before 5.00.9813 (2023) misload numbers above 255 (323 load correctly in 5.00.10305) | P: FolderName, one folder per map (WinOLS 2.08+) | Y |
| Identity | Project / ECU metadata | P: name and version parsed; most header fields unknown | P: same | Y: PROJECT, HEADER, MODULE, MOD_PAR (EPK, CPU) | P | P: title, description, author | N | Y |
| Addressing | Address is a file offset | Y | Y | N: ECU CPU address; needs a segment-to-file mapping | N: ECU address | Y: plus header baseoffset | Y: StartAddr / DataAddr (hexdump address, decimal) | Y: file offset plus original CPU address and segment |
| Addressing | Memory segments | N | N | Y: MEMORY_SEGMENT | ? | P: REGION (size only) | N | Y |
| Addressing | Checksum definition | N: separate WinOLS checksum modules | N | P: vendor-specific IF_DATA | N | Y: CHECKSUM (plugin) | N | Y |
| Addressing | Search signature (find map in other SW) | Y: per map and per axis | Y | N | N | N | P: search commands place maps at import; not stored with the map | Y |
| Data layout | Single value / 1D / 2D | Y | Y | Y: VALUE / CURVE / MAP | Y: Festwert / Kennlinie / Kennfeld | Y: CONSTANT / TABLE | Y: Typ eEinzel / eEindim / eZweidim | Y |
| Data layout | 3D and higher | N | N | Y: CUBOID, CUBE_4, CUBE_5 | N | N | N | Y |
| Data layout | Value block (array, no axes) | P: 2D with ordinal 1,2,3 axes | P: same | Y: VAL_BLK | ? | Y: table with label axes | P: 2D with no-axis data source | Y |
| Data layout | Strings | N | N | Y: ASCII | ? | P: outputtype 4 | N | Y |
| Data layout | Integer 8/16/32, signed | Y | Y | Y: also 64-bit | Y | Y | Y: DataOrg + bVorzeichen | Y |
| Data layout | Float | Y: 32-bit only | Y: 32-bit only | Y: 16/32/64-bit | ? | Y: float flag, 32-bit; 64-bit unconfirmed. Added in 5.00.8008 (2012) with no equations on float data (only `X`); whether that limit still holds is unconfirmed | Y: eFloatLoHi / eFloatHiLo, 32-bit | Y |
| Data layout | Endianness per object | Y: HiLo/LoHi in type enum | Y | Y: BYTE_ORDER, module default | ?: likely global | Y: mmedtypeflags | Y: DataOrg | Y |
| Data layout | Row vs column major | Y: '2d Inverse' (252 of 4170 maps in 8D0907551G) | Y: '2d Inverse' (99 of 471 in 8D0907551M) | Y: RECORD_LAYOUT | ? | Y: major stride / flags | Y: Typ eZweiInv | Y |
| Data layout | Gaps, alignment, interleaved records | N | N | Y: RECORD_LAYOUT positions, ALIGNMENT_* | ? | P: stride only | P: SkipBytes on axes only | Y |
| Data layout | Bit mask / sub-byte fields | ?: not decoded; may be in an unknown block | ? | Y: BIT_MASK | ? | P: XDFFLAG, single bit | N | Y |
| Conversion | Linear factor + offset | Y | Y | Y: LINEAR / RAT_FUNC | Y | Y: equation | Y: Faktor / Offset (help example uses a decimal comma; locale to test) | Y |
| Conversion | Reciprocal (f / x) | Y: flag | Y: flag | Y: RAT_FUNC | ? | Y: equation | Y: bKehrwert | Y |
| Conversion | General rational (6 coefficients) | N | N | Y: RAT_FUNC | ? | Y: equation | N | Y |
| Conversion | Arbitrary formula | N | N | Y: FORMULA (+ inverse) | N | Y: equation; can reference other objects | N | Y |
| Conversion | Enum / lookup tables | N | N | Y: COMPU_TAB, COMPU_VTAB, VTAB_RANGE | ? | N | N | Y |
| Conversion | Units | Y | Y | Y: COMPU_METHOD unit / PHYS_UNIT | Y | Y | Y: Einheit | Y |
| Conversion | Display precision | Y | Y | Y: FORMAT | ? | Y: decimalpl | Y: Nachkommastellen | Y |
| Conversion | Display base dec / hex / binary | Y: base field | Y: base 10/16/2; binary on codewords, e.g. CWMSRCAN | N | ? | P: dec/hex only | P: Radix 10 / 16; binary not documented | Y |
| Conversion | Min / max limits | ?: range field is 0-255 on every map | ?: range varies per map (-13..42, 2..46); edit limits or stored data range? | Y: limits + EXTENDED_LIMITS | ? | Y: min / max | N: no property | Y |
| Conversion | Difference / percent view vs original | Y: flags | Y: flags D, P | N | N | N | Y: bDelta / bProzent | Y |
| Conversion | Unidentified per-map doubles | ?: header9a/10/11 hold doubles: 300.0, 1.0, -0.9 | ?: same blocks; 0.01, -2.7 also seen | - | - | - | - | Y: kept as raw hex in the per-source block until identified |
| Axes | Axis read from image | Y: datasource EEPROM | Y | Y: STD_AXIS / COM_AXIS | Y | Y: EMBEDDEDDATA | Y: DataSrc eRom | Y |
| Axes | Per-axis type, sign, units, conversion | Y | Y | Y | Y | Y | Y: StuetzX/Y.* | Y |
| Axes | Fixed axis values (not in image) | Y: '1,2,3' ordinal or 'Free editable'; axis block sized 4 bytes per point | Y: '1,2,3' ordinal; free-editable storage not located | Y: FIX_AXIS_PAR / _DIST / _LIST | ? | Y: static LABELs | P: DataSrc eUserdef, but no property for the values | Y |
| Axes | Shared axis object | N: address repeated per map | N | Y: COM_AXIS -> AXIS_PTS | ? | Y: axis linked to another table (`embedinfo type="3" linkobjid`, the table's uniqueid; TunerPro 5) | N | Y |
| Axes | Axis point count read from image | ? | ? | Y: NO_AXIS_PTS in record layout | ? | N | ?: DataHeader marks header bytes; whether the count is read is undocumented | Y |
| Axes | Axis stored as differences | Y: 'EEPROM, subtract' = gaps between breakpoints, counted down from 256 (`kp-format.md`); 273 axes in 8D0907558E; 'add' and 'backwards' still unknown | Y: same datasource field; ME7 packs have mislabelled absolute axes | Y: DEPOSIT DIFFERENCE | ? | P: static LABELs computed from the bin; or live, through a linked helper table with `CELL()` equations (`kp-format.md`; XML unconfirmed) | Y: DataSrc eRomAdd / eRomSub; also eRomBackwards | Y |
| Axes | Rescale / curve axes | N | N | Y: RES_AXIS, CURVE_AXIS | N | P: axis "Normalized" through a function (XDFFUNCTION, Ford-style normalizer); how closely it matches CURVE_AXIS is unconfirmed | N | Y |
| Tool-specific | Patches | N | N | N | N | Y: XDFPATCH | N | Y |
| Tool-specific | Measurements (logging channels) | N | N | Y: MEASUREMENT; useful for ecuxplot | N | N | N | Y |

## Totals

| Format | Y | P | N | ? |
|---|---|---|---|---|
| KP v1 | 20 | 2 | 13 | 4 |
| KP v2 | 21 | 2 | 12 | 4 |
| A2L | 31 | 2 | 5 | 0 |
| DAMOS | 8 | 2 | 9 | 19 |
| XDF | 22 | 10 | 6 | 0 |
| WinOLS script | 17 | 6 | 14 | 1 |
| JSON/YAML | 39 | 0 | 0 | 0 |
