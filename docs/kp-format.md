# KP format ("OLS Map Pack", *.kp)

What is known about WinOLS map packs, as read by ecuxplot's mapdump (`org.nyet.mappack` in [ecuxplot](https://github.com/nyetlabs/ecuxplot)) and the Go port in `../kp/`. All integers are little-endian.

Fields whose meaning isn't known get the most likely name, marked unconfirmed here and "(unconfirmed)" in the Go comment. Constants with no known meaning are named after the field they follow, with `Pad` (zeros), `Flag` or `Tag` (`IDPad`, `BuildFlag`, `NameTag`). Raw regions are named for the region (`FixedHeader`, `HeaderBody`, `Trailing`). JSON names start lower-case (`view3DRotation`); `kp-defaults.json` uses them (`model.md`).

These names replace mapdump's `h`, `h1`, `h9a`, which split the map tail differently; a field is renamed when a WinOLS save shows its meaning.

Values below come from a survey (`XDFKIT_SURVEY=out.md go test ./kp -run TestSurvey`) of the unconfirmed fields over the 13 archived packs, `test8maps.kp` and the WinOLS 2.24 saves in `testdata/local/`, with their images: values per layout, how many files each varies within, and which named fields it equals or changes with. Constant everywhere, so only a WinOLS save can tell more: `EndTag`, project `NameTag`, `VersionTag`, `Build`, `MapsTag`, folder `Flags` and `Style`, map `IDPad`, `IDFlag`, `ImageSizePad`, `YPad`, axis `MirrorPad`.

## Versions

| Layout | Header id at 0x10 | Value at 0x14 | Fixed header ends | Maps stored |
|---|---|---|---|---|
| v1 | 0x71, 0x74 | 0 | 0x5c | inline |
| v2 | 0x124, 0x149 | total file length | 0x60 | deflated zip entry `intern` |

- All 11 v2 packs in ecuxplot's data use 0x124; 0x149 is accepted by mapdump but not seen in them.
- Every KP in ecuxplot's data ends with the same 4 bytes `33 88 72 98`, so that is a constant, not a checksum.
- WinOLS 2.24 is `2.24.00` (string in `ols_32on32.exe`). It writes v2 with header id 0x124 and the version string "OLS 5.0 (Windows)" (see WinOLS 2.24 exports).
- WinOLS 5 KP files are known not to load in WinOLS 4. The free WinOLS 5 demo (5.87, Windows 11 ARM under UTM) imports v1 and v2 KP files, including ones written by xdfkit (confirmed). The demo can't save anything, so it can check imports but produce no samples.

## Strings

- int32 length, then that many ISO-8859-15 bytes, then one NUL byte.
- A zero length has no NUL byte.

## File layout

- Signature string ("WinOLS File"), int32 header id, int32 header length.
- Seek to the fixed header end (0x5c or 0x60): filename string, version string.
- Undecoded header, with three int32 `-1` terminators (four in v2), kept raw as `File.HeaderBody`. The exact walk is in `kp.walkHeader` and mapdump's `Parser.parseHeader`. The bytes from 0x18 (after the header length) to the fixed header end are kept raw as `File.FixedHeader` (zeros).
- The last 16 bytes of the header, in both layouts: int32 `File.EndOffset`, the absolute offset of the end marker `33 88 72 98` (file length - 4; true in all 16 ecuxplot packs), then 12 bytes kept raw as `File.EndTag` (`00000000 99780042 02000000` in every pack; a format tag, unconfirmed). WinOLS 2.24 uses the offset: a file whose length changed without updating it imports, but raises WinOLS's crash-report dialog.
- Project: name string, int32[4] `NameTag`, (v2: 4 bytes `NamePad`, 0), version string, int32[4] `VersionTag` (equal to `NameTag`), (v2: 4 bytes `VersionPad`, 0), int32 `Build`, (v2: one byte `BuildFlag`, 1). Each holds the same value in every pack and WinOLS 2.24 save; the tags are type tags and `Build` a build or format id (unconfirmed).
- Maps:
  - v1: int32 count, then the map records.
  - v2: int32 zip length, then that many bytes of zip archive, then the folder table follows directly. (mapdump's `Project.java` rewinds to `start + zsize` and reads one int, which nets out to no extra field.) The zip holds one entry `intern`; its inflated content is one byte `InternFlag` (0), int32 count, then the map records.
- int32[3] `MapsTag` (the same in every pack; first and third equal; a type tag, unconfirmed).
- Folders: int32 count, then per folder int32 id, int32 `BuiltIn` (9000 on the "My maps" folder and 9001 on "Hexdump" in every pack, 0 on all others), name string, 2 bytes `Flags` (1, 1; expanded and visible, unconfirmed), int32 `Style` (0x1000000), (v2: 15 zero bytes `Pad`). mapdump sorts folders by name, drops duplicate names and renumbers map folder ids; the Go reader keeps them raw.
- Trailing bytes (not decoded; kept raw as `Project.Trailing`).

## Round trip

`kp.Parse` and `File.Encode` share one layout walk (`kp/codec.go`), and every byte is either a field or a raw `Hex` block. The record layouts are the Go structs themselves: fields are coded in declaration order by reflection, with `kp:"..."` struct tags for layout-only fields (`v1`, `v2`), raw lengths (`len=N`), count- or byte-length-prefixed lists (`count`, `bytes`), the trailing bytes (`rest`) and hand-coded fields (`hook`: the fixed header, the header walk and the v2 map block). The tag grammar is documented at the top of `codec.go`.

`File.Offsets(paths...)` (and single-path `File.Offset`) returns where fields are stored by JSON path (`project.maps[3].x.dataSource`), in the file or the inflated v2 map block; currently unused. Decoding fails on anything that couldn't be re-encoded identically: a bool byte other than 0/1, a string without its NUL, an axis list length not a multiple of 4, bytes left after the v2 maps. Encode recomputes the v2 length at 0x14 and `EndOffset` and stores them back into the `File`.

- Parse then Encode is byte-identical for all 16 ecuxplot packs. For v2 the original zip is reused while the map block is unchanged.
- KP to JSON to KP: byte-identical for v1. For v2 the map block is re-deflated (Go `compress/flate`, level 9). The inflated block, everything before the zip (except 0x14 and `EndOffset`) and everything after it are identical. Re-deflated zips land within about 0.4% of the original size.
- Editing a string length (which shifts every later offset) and an address re-encodes and parses back with only those changes.

## v2 zip container

Verified on all 11 v2 packs in ecuxplot's data:

- One local file entry `intern`, method 8 (deflate), general-purpose flag 2, DOS date and time zero, version 20.
- The stored stream is reproduced exactly by zlib 1.2.x raw deflate at level 9, memLevel 8, window 15. WinOLS 2.24 links zlib 1.2.3 statically.
- xdfkit uses Go `compress/flate`, whose stream differs, so the zip length, the length at 0x14 and `EndOffset` must be updated. WinOLS 2.24 imports such files (re-zipped and lint-fixed 8D0907551M); with a stale `EndOffset` it raises its crash-report dialog.

## WinOLS 2.24 exports

8D0907551M.kp was imported into WinOLS 2.24 and exported four times: unchanged, unchanged again, with KFWKSTAB X set to "EEPROM", and with KFLDRQ2 also renamed. A new project with no maps was exported as well. Compared with the original pack:

- Header id 0x124, version string "OLS 5.0 (Windows)". The filename string is the name the file was exported under, usually upper-cased (`W1.KP`).
- The project's `NameTag`, `VersionTag`, `Build` and `MapsTag` are identical in every export, including the empty project.
- `File.HeaderBody` differs in two runs, 15 bytes at offset 0x3a and 68 bytes at 0x76 within the block, and two bytes of the first run change on every save. The contents look like uninitialized memory: MSVC heap fill words (`0xbaadf00d`), stack addresses around 0x0019d000 and DLL addresses around 0x73000000. Treat these bytes as don't-care.
- `Project.Trailing` is the same kind of thing: 1214 bytes differ between the original and an export, about 40 between consecutive exports, and most of the changed words look like pointers. A few small values change as well, with unknown meaning.
- Map end address: WinOLS writes start + byte length (one past the last byte) and recomputed it on 13 maps of the original whose stored ends were stale (TVCAMSR: start 90912, end 91094, rewritten as 90914). Stale ends are common in hand-edited packs (4Z7907551AA, 8D0907551K/M, 8D0907558M); lint rule R8 recomputes them (`autocorrect.md`), giving the same values as WinOLS on 8D0907551M.
- Map `Cursor` (int32[2]) changes only on maps opened during the session: KFWKSTAB went from 1, 1 to -1, 0 after its axis edit, the renamed map from 3, 3 to 1, 0, and KFNLLNST from -1, 2 to 0, 2 on import. Most values lie within the map's dimensions, so it is taken as the last cursor cell (unconfirmed).
- WinOLS stores per-map view state in the map record: `ViewMode` is the script's `ViewMode`, and map `Selected` and `ViewScale` change with use. The right-hand window setting (the script's `RWin`: hex, bars) is taken as `RightPane` (unconfirmed; the right-pane saves were made in the hexdump, not the map window). Where "original values" (the script's `bOriginalWerte`: the original version's values instead of the current version's) and the raw display (`bOriginal`: factor and offset ignored) are stored, if at all, is unknown. The map flags `reciprocal`, `signed`, `difference` and `percent` are in the script's order (`bKehrwert`, `bVorzeichen`, `bDelta`, `bProzent`), so taken as the same settings (unconfirmed); the map window's difference and percent toggles would then be `difference` and `percent`. The hexdump has its own set of these toggles; turning on its "original values" changed no KP field in WinOLS 2.24, so the hexdump's toggles are taken as project state that the KP doesn't hold (unconfirmed).
- The `intern` stream of every export is reproduced exactly by zlib at level 9, memLevel 8, as with the ecuxplot packs.
- Exporting twice under one name appended the second export to the first (saved over RDP drive redirection to macOS, where a TunerPro save also left an old tail, so the redirection is the cause, unconfirmed). Then 0x14 holds the combined length, `EndOffset` the first copy's marker, and `kp.Parse` reads the first copy and keeps the rest in `Trailing`.

## Map record

Field order (v2-only fields marked), as declared in `kp.Map`:

- byte `Selected` (0; 1 on a few dozen maps per v2 pack, and a different set in each WinOLS export of the same project, so editor state; taken as the selection in the map list, unconfirmed); v2: int32 `LinkID` (-1 in every map; a linked map's id, unconfirmed), comment string, byte `CommentFlag` (0)
- name string (long description)
- int32 organisation, int32 `RightPane` (2 in most packs; 0 on every map of 06A906032HS, 3 on most of 4D1907558, a few 0, 1 or 3 elsewhere; taken as the script's `RWin`, the right pane in text mode, 0 none, 1 hex, 2 bars, 3 both, since the values fit; unconfirmed), int32 value type, int32 width, int32 display base, int32 folder id. The width equals the value type's width in bytes (1, 2, 4) on all 32,315 maps in ecuxplot's packs, so it is the element size (unconfirmed which of the two WinOLS reads).
- id string (short identifier; mapdump strips anything after a space or `?`)
- int32 `IDPad` (0), byte `IDFlag` (0); v2: int32 `Marked` (0 or 1), taken as the map list marker: WinOLS 2.24 has "Insert/delete marker" and the map list filter "Markers: Only marked / Only unmarked" (`OLS_LangE.dll`), and which field holds the marker is unconfirmed. It is mostly the same within a pack; where it varies it follows the author's choices (4D1907558: 18 of 538, the "(AR ...)" ports; 8D0907551M: 439 of 471, the unmarked ones axis helpers, DTC and unnamed maps). WinOLS 2.24 takes it from an imported KP into the project and writes it back on export (a project that imported a KP with every map at 1 exported every map at 1). A map duplicated in WinOLS 2.24 gets the original's value, so it is not a copy marker. Kept in the model as `marked`; KP output without it writes 0.
- int64[3][2] `range`: the value range in the map properties ("Value range:" on the Map tab), low and high, one pair per element width: 8-, 16- and 32-bit. Setting 10 to 90 on TLDOBAN (8-bit) in WinOLS 2.24 wrote the first pair, and 1000 to 6000 on NMAX (16-bit, factor 0.25) the second, as typed (confirmed); the 32-bit pair is unconfirmed. The dialog shows the stored numbers unchanged (NMAX's default reads 0 to 65535, not scaled by the factor), so the range is in raw units, or in whatever units the map display is set to (unconfirmed). The defaults are 0 and 255, 0 and 65535, 0 and 4294967295. Negative lows are sign-extended (8D0907551M KFZW2: -18, 47).
  - First pair: other values (0-256, 0-151, 22-178, 10-90) mostly in hand-edited packs, often copied from map to map with the properties. On 8D0907551M KFZW, KFZW2 and KFZWOP it is the displayed minimum and maximum of the data, truncated (KFZW2: raw -24 to 63, factor 0.75, range -18 to 47).
  - Second pair: on hand-edited maps other pairs, such as 170 and 8149 (the raw minimum and maximum of KFMIRL's data in 8D0907551H, copied to 8-bit maps with the map properties), or 0 and the raw maximum of the map's own data (8D0907551M KFPEDR_0_A 32768, KFPRG 8192).
  - Third pair: the low is 0, or 52 or 2621440 on a few maps; the high is 4294967295, or 4294967296 on maps the user edited (mostly with the second high at 65536).
- bytes: reciprocal, signed, difference, percent
- int32 columns, int32 rows (patchable)
- int32[2] `Cursor` (editor state, see WinOLS 2.24 exports), int32 precision
- value block: description string, units string, float64 factor, float64 offset
- int32 start address, int32 end address (start + byte length as WinOLS writes it; packs can hold stale values), int32 image size (0x100000 in most packs, 0x20000 in 8D0907558E, 0x40000 in 8D0907558M: the image length in all 16 ecuxplot packs, so taken as the image size); v2: int32[2] `ImageSizePad` (0)
- int32 `addr2` (equals the start address on most maps, and on every map in 9 of the 16 packs; moving a map or duplicating it to a new address in WinOLS 2.24 sets it to the new start address. It differs only in hand-edited packs (4D1907558, 4Z7907551AA, 8D0907551K, 8D0907551M, 8D0907558M), where the end address often equals `addr2` plus the byte length instead of the start plus it (8D0907551K 108 of 112 such maps, 8D0907558M 385 of 402), so `addr2` and the end address are taken as the location as of the last properties edit, and some other kind of move updated only the start; unconfirmed), int32[2] `Addr2Flags` (first -1 in every map of 8D0907551F, G, H, K and test8maps, else 0; second always 0), int32 `Addr2ImageSize` (the image length, or 0 in every map of a few packs; unconfirmed), int32 `Addr2Pad` (0)
- X axis record, Y axis record
- The tail, 80 bytes in both layouts. Its fields and the values seen in ecuxplot's 32,315 maps:

| Offset | Field | Type | Values |
|---|---|---|---|
| 0x00 | `YFlag` | int32 | 0; 1 on TNMXH (8D0907551M) and KFLDS.0 (8D0907558M) |
| 0x04 | `YPad` | int16 | 0 |
| 0x06 | `StoredCols` | int32 | columns in storage order, see below |
| 0x0A | `StoredRows` | int32 | rows in storage order |
| 0x0E | `ViewMode` | int32 | view mode: 1 text, 2 2D, 3 3D (confirmed in WinOLS 2.24); the packs hold 1 or 3 (3 on most of 8D0907551G and some hand-edited maps) |
| 0x12 | `ViewFlags` | byte[2] | 1, 1 |
| 0x14 | `View2DScale` | float64[2] | 1.0, 1.0 on all but a few dozen maps, all in hand-edited packs (first 2, 8 or 64 there); taken as the 2D view's scaling (unconfirmed) |
| 0x24 | `View2DRef` | int32 | -1 on all but about 80 maps (0, or values like 44415); WinOLS 2.24 sets it to 0 when the view mode is first changed |
| 0x28 | `ViewFlag` | byte | 1; 0 on 5 maps in 4D1907558 and 8D0907551M |
| 0x29 | `View3DRotation` | float64[2] | 300.0, 1.0 on most maps; 0.0, 0.01 on about 1700, many of them 1D maps in 06A906032HS; other pairs in hand-edited packs; taken as the 3D view's angle and zoom (unconfirmed) |
| 0x39 | `ViewScale` | float64 | 0.0 on most maps, see below |
| 0x41 | `ViewScaleRef` | int32 | -1 exactly where `ViewScale` is 0.0, else 0 or a value up to about 80000 |
| 0x45 | `ViewOffset` | float64 | 0.0 or -0.0 on most maps, see below |
| 0x4D | `Term2` | byte[3] | 1, 1, 1; the first two must be 1 |

`StoredCols` and `StoredRows` are columns then rows, swapped on "2d Inverse" (column-major) maps: true on every map in the archived packs and the WinOLS 2.24 saves except 11 single values. xdfkit writes them that way.

`ViewScale`, `ViewScaleRef` and `ViewOffset` are taken as the display's automatic scaling (unconfirmed). WinOLS 2.24 stores them when a map is first shown in the 3D view, not when it is opened in text or 2D (TLDOBAN: 51.2, 33232, -7.2). They are non-default on exactly the same maps, mostly in hand-edited packs, and on single values follow the value: `ViewScale` is 2560 divided by a multiple of the value and `ViewOffset` -0.9 times that multiple (8D0907551M: CATR, value 1, 2560 and -0.9; CLAHSH, value 3, 853.33 and -2.7; CWGGPBKV, value 6, 426.67 and -5.4). They are not a second factor and offset. mapdump reads the tail as int32 and int16 chunks that straddle these floats.

## Axis record

- value block (description, units, factor, offset)
- int32 data source (patchable)
- int32 address (patchable; meaningful only for image data sources)
- int32 value type, int32 width, int32 display base; mirror map flag (v2: int32, v1: byte; 0 or 1); v2: 9 zero bytes `MirrorPad`. The width equals the value type's width in bytes on all but 2 of the 64,630 axis records in ecuxplot's packs (NMAX and NMAXAL in 06A906032LP: single values with a stray defined u8 axis and width 4), so it is the element size, like the map's.
- byte reciprocal, int32 precision (-1 on the unused slots of 06A906032HS only), byte signed (patchable)
- int32 byte length, then that many bytes of `Values`: the values of a "Free editable" axis (datasource 4), 4 bytes per point, the values first. Setting TLDOBAN's 8-bit X axis to free editable with 1 to 8 in WinOLS 2.24 wrote 32 bytes, `01 02 ... 08` then zeros, and cleared the axis address and factor (to 0 and 1); switching it to "Eprom, backwards" emptied the list again (confirmed). So the values are packed in the axis's element type, at least for 8-bit axes (16- and 32-bit unconfirmed). In v1 both axis records of a map hold zeros, 4 bytes per stored row, on almost every map; in v2 the list is empty except on free editable axes (LDRXN's two Y axes in 4D1907558: 16 zeros).
- int32 `DataHeader`: the script property `DataHeader` (unconfirmed), the number of header bytes before the axis data (surveyed over the 12 OEM pack/image pairs): it is 0, 1, 2 or 4, and where it isn't 0 the bytes just before the axis hold the point count in 215 of 217 1-byte headers on byte axes, 544 of 619 2-byte headers on byte axes (the M3.82 [variable id][count] header gives 2 on 462 of 468 such axes in 8D0907558E) and 113 of 130 2-byte headers on word axes. 4-byte headers on word axes match only 28 of 84 ([X count][Y count], unconfirmed). Many axes with a header in the image have 0, so it is a setting, not detected.
- int32 `SignatureByte`: the script property `SignaturByte` (marker byte before the axis, `0xFFFFFFFF` for none; `winols-script.md`; unconfirmed). -1 is the commonest value in every pack. In 8D0907558E (M3.82), 468 of the 477 image axes with another value have that byte two bytes before the axis, which is the variable id of the [variable id][count] header; the match is weaker in 8D0907558M (49 of 311) and in the ME7 packs, so the exact rule is unconfirmed.
- Every map stores both axis records; the organisation says which are used (X for 1D, X and Y for 2D, none for a single value), and WinOLS 2.24 goes by it alone (confirmed: NMAXAL in 06A906032LP, a single value with eeprom slots, shows no axes). Unused slots often have the mirror flag set (and precision -1 in v2).
- Mirror flag: WinOLS 2.24's "&Mirror map" axis setting (`OLS_LangE.dll`), not an "undefined" marker; taken as the script's `bRueckwaerts` (unconfirmed). Set on every X axis of 8D0907551F (724), 24 in 8D0907551K, 4 in 8D0907551H. Storage is not reversed (those axes increase in the image like the others); WinOLS shows the axis and the cells along it descending (8D0907551F KFZW X: stored 512 to 8534, shown 8534 down; confirmed). mapdump treats the flag as "undefined" and drops those axes' units and scale, so most archived CSVs differ from xdfkit's in those columns.

## Enums

| Field | Values |
|---|---|
| Organisation | 2 single value, 3 one-dimensional, 4 two-dimensional, 5 2D inverse |
| Value type | 1 8-bit, 2 16-bit HiLo, 3 16-bit LoHi, 4 32-bit HiLo, 5 32-bit LoHi, 6 float HiLo, 7 float LoHi |
| Axis data source | 0 ordinal ("1, 2, 3, ..."), 1 EEPROM, 2 EEPROM add, 3 EEPROM subtract, 4 free editable, 5 EEPROM backwards |
| Display base | 10, 16, 2 |

- 4 and 5 follow the script help's enum order, eDataSrcNone, eRom, eRomAdd, eRomSub, eUserdef, eRomBackwards (confirmed in WinOLS 2.24: "Free editable" saved 4, "Eprom, backwards" 5). `OLS_LangE.dll` lists them in UI order instead: "1, 2, 3, ...", "Eprom", "Eprom, add", "Eprom, subtract", "Eprom, backwards", "Free editable". No defined axis in the ecuxplot packs uses 4 or 5; 4 appears only on unused axis slots in 4D1907558.
- "2D inverse": mapdump's row-major reading is supported by a smoothness check against the bins (34 maps to 1, 50 ties). XDF Porter marks these column-major; don't copy that.

## Axis datasource "EEPROM, subtract"

The stored values are offsets between breakpoints, counted down from the top of the value range.

- Semantics (confirmed in WinOLS 2.24): for raw values raw[0] to raw[n-1], WinOLS displays point i = conversion(256 - (raw[i] + raw[i+1] + ... + raw[n-1])) for 8-bit axes, without wrapping. Each raw value is the gap to the next point, and the last one is the gap to 256. Checked on two axes:
  - FHSA.0 X in 8D0907558E (raw 27, 13, 12, 60, conversion 0.75*X-48) displays 60, 80.25, 90, 99 (raw 144, 171, 184, 196).
  - KFWKSTAB X in 8D0907551M (raw 24, 49, 50, 164, total 287) displays -71.25, -53.25, -16.50, 21.00 (raw -31, -7, 42, 92). The negative first point is how a mislabelled absolute axis shows (below).
  - 16-bit "subtract" axes are counted from 65536 (unconfirmed; none seen).
- Evidence that this is the ECU's meaning, not only WinOLS's (8D0907558E/M, 128 KB Motronic images with axes around 0x8xxx, 273 and 285 axis entries labelled "subtract" in ecuxplot's packs, 506 and 361 in the corpus JSON with lint R2 applied): read this way, every "subtract" axis in 8D0907558E increases from 0 or more, and the breakpoints are round numbers in the operating range:
  - `TLAN` (rpm, 40*X): 1840, 2000, 3000, 4000, 4520, 5520, 6000, 6800
  - `KFZW.0` Y (rpm): 600, 800, 1000, 1480, ..., 5520, 6000, 6800
  - `FKHE.0` (%, 0.390625*X): 0, 1.95, 25, 75, 99.61
  - `TADTEVT` (s, 0.5*X): 0, 4, 6.5, 9, 15, 18, 21, 25
  - `FHSA.0` ("Heißstartanhebung", hot-start enrichment, degC): 60, 80.25, 90, 99
  - A forward running sum also increases but gives odd values (TLAN 160 to 8400 rpm), so it is wrong.
- Layout: the two bytes before these axes are [input variable id][point count], e.g. `[164, 8]` on rpm axes, `[240, n]` on % axes, `[155, n]` on temperature. That's consistent with Motronic M3.x-style axis headers; ME7 axes have only the count byte.
- Pack errors exist in both directions, so the label can't be trusted blindly:
  - ecuxplot's 8D0907558E and 8D0907558M packs have "subtract" axes labelled plain "EEPROM" (KFFA X: raw 12, 12, 14, 16, 17, 13, ..., 36, which read as "subtract" is a clean 1.0, 1.6, 2.2, ..., 10.5, 11.0 ms/rev, as WinOLS 2.24 displays it once relabelled). The corpus JSON has them relabelled (lint R2).
  - The 27 "subtract" axes in the ME7 packs (8D0907551G/M) are absolute and mislabelled by the pack author: read as "subtract" their totals exceed 256, so they start below zero (KFWKSTAB above). With the datasource set to "EEPROM" (`xdfkit fix`, rule R1), WinOLS 2.24 displays KFWKSTAB as -30, -11.25, -10.5, 75, the plain values.
- Implementation:
  - KP reader/writer: the model keeps the datasource exactly as stored (`axis.stored` = absolute / add / subtract / backwards), never "corrected".
  - XDF writer: an axis equation can't sum over following cells, so xdfkit writes static LABELs computed from `-i image` (without an image, a plain image axis), as mapdump does; TunerPro shows them correctly (FHSA.0, TLAN in 8D0907558E). Live alternative for TunerPro 5 ([forum](https://tunerpro.net/forum/viewtopic.php?t=3672)): a helper table at the axis address with per-row equations (last row `(256 - X) * factor + offset`, others `CELL(ROW()+1;0;FALSE) - X * factor`), linked as the map's axis (`embedinfo type="3" linkobjid`); its XDF encoding is unconfirmed, and builds before 5.00.8414 (2014) miscompute that pattern.
  - Lint: rules R1, R2 and R6 (`autocorrect.md`) flag axes whose label disagrees with the data.
