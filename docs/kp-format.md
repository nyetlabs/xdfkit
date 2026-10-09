# KP format ("OLS Map Pack", *.kp)

What is known about WinOLS map packs, as read by ecuxplot's mapdump (`org.nyet.mappack` in [ecuxplot](https://github.com/nyetlabs/ecuxplot)) and the Go port in `../kp/`. All integers are little-endian.

Undecoded fields are named after the nearest preceding named field, plus `Unk`, plus their byte offset from the end of that field in uppercase hex, counted in the v2 layout (v1 lacks the v2-only fields but keeps the names): `StoredRowsUnk1B` starts 0x1B bytes after the map's `StoredRows`. A field named later re-anchors the placeholders after it (`YUnk29` became `StoredRowsUnk1B`). With nothing named before them, the offset is from the start of the file (`Unk18`) or of the record (map `Unk00`). After an unnamed variable-length list the anchor is the list (`ListUnk00`, until it was named `DataHeader`). Raw regions are named for the region (`UnkHeader`, `UnkIntern`, `UnkTrailing`). JSON names start lower-case (`storedRowsUnk1B`); `kp-defaults.json` uses them (`model.md`).

`Unk` marks a placeholder: each should get a real name once its meaning is known. mapdump's `Parser.java` calls these fields `h`, `h1`, `h9a` and so on, and splits the map tail at different boundaries; the Go names replace those. A few named fields are still guesses and say so (`range`, `addr2`, `addr2ImageSize`, `signatureByte`, `viewScale`).

The values quoted below for the placeholders come from a survey of the 13 archived packs (dated copies left out), `test8maps.kp` and the WinOLS 2.24 saves in `testdata/local/`, with their images: `XDFKIT_SURVEY=out.md go test ./kp -run TestSurvey` reports, per placeholder, the values per layout, how many files it varies within, and on records where it isn't the commonest value the named fields it equals and the other fields that change with it. Placeholders that are constant everywhere (`EndOffsetUnk00`, the project's `NameUnk*`, `VersionUnk*` and `MapsUnk00`, the folder's `NameUnk*`, map `IDUnk00`, `IDUnk04`, `ImageSizeUnk00`, `YUnk04`, axis `MirrorUnk00`) say nothing more until a WinOLS save changes them.

## Versions

| Layout | Header id at 0x10 | Value at 0x14 | Fixed header ends | Maps stored |
|---|---|---|---|---|
| v1 | 0x71, 0x74 | 0 | 0x5c | inline |
| v2 | 0x124, 0x149 | total file length | 0x60 | deflated zip entry `intern` |

- All 11 v2 packs in ecuxplot's data use 0x124; 0x149 is accepted by mapdump but not seen in them.
- Every KP in ecuxplot's data ends with the same 4 bytes `33 88 72 98`, so that is a constant, not a checksum.
- WinOLS 2.24 is `2.24.00` (string in `ols_32on32.exe`). It writes v2 with header id 0x124 and the version string "OLS 5.0 (Windows)" (see WinOLS 2.24 exports).
- WinOLS 5 KP files are known not to load in WinOLS 4. The free WinOLS 5 demo (5.87, Windows 11 ARM under UTM) imports v1 and v2 KP files, including ones written by xdfkit (confirmed 2026-10-07). The demo can't save anything, so it can check imports but produce no samples.

## Strings

- int32 length, then that many ISO-8859-15 bytes, then one NUL byte.
- A zero length has no NUL byte.

## File layout

- Signature string ("WinOLS File"), int32 header id, int32 header length.
- Seek to the fixed header end (0x5c or 0x60): filename string, version string.
- Undecoded header, with three int32 `-1` terminators (four in v2), kept raw as `File.UnkHeader`. The exact walk is in `kp.walkHeader` and mapdump's `Parser.parseHeader`. The bytes from 0x18 (after the header length) to the fixed header end are kept raw as `File.Unk18`.
- The last 16 bytes of the header, in both layouts: int32 `File.EndOffset`, the absolute offset of the end marker `33 88 72 98` (file length - 4; true in all 16 ecuxplot packs), then 12 bytes kept raw as `File.EndOffsetUnk00` (`00000000 99780042 02000000` in every pack). WinOLS 2.24 uses the offset: a file whose length changed without updating it imports, but raises WinOLS's crash-report dialog (2026-10-07).
- Project: name string, int32[4] `NameUnk00`, (v2: 4 bytes `NameUnk10`, 0), version string, int32[4] `VersionUnk00`, (v2: 4 bytes `VersionUnk10`, 0), int32 `VersionUnk14`, (v2: one byte `VersionUnk18`). Each holds the same value in every ecuxplot pack.
- Maps:
  - v1: int32 count, then the map records.
  - v2: int32 zip length, then that many bytes of zip archive, then the folder table follows directly. (mapdump's `Project.java` rewinds to `start + zsize` and reads one int, which nets out to no extra field.) The zip holds one entry `intern`; its inflated content is one byte, int32 count, then the map records.
- int32[3] `MapsUnk00` (the same in every pack).
- Folders: int32 count, then per folder int32 id, int32 `BuiltIn` (9000 on the "My maps" folder and 9001 on "Hexdump" in every pack, 0 on all others), name string, 2 bytes `NameUnk00` (1, 1), int32 `NameUnk02` (0x1000000), (v2: 15 zero bytes `NameUnk06`). mapdump sorts folders by name, drops duplicate names and renumbers map folder ids; the Go reader keeps them raw.
- Trailing bytes (not decoded; kept raw as `Project.UnkTrailing`).

## Round trip

`kp.Parse` and `File.Encode` share one layout walk (`kp/codec.go`), and every byte is either a field or a raw `Hex` block. The record layouts are the Go structs themselves: fields are coded in declaration order by reflection, with `kp:"..."` struct tags for layout-only fields (`v1`, `v2`), raw lengths (`len=N`), count- or byte-length-prefixed lists (`count`, `bytes`), the trailing bytes (`rest`) and hand-coded fields (`hook`: the fixed header, the header walk and the v2 map block). The tag grammar is documented at the top of `codec.go`.

`File.Offsets(paths...)` returns where fields are stored, by JSON path (`project.maps[3].x.dataSource`), in one walk: in the file, or for v2 maps in the inflated map block. `File.Offset(path)` is the single-path form. It was built for a byte-patching writer, which the round-trip writer made unnecessary; nothing uses it yet. Decoding fails on anything that couldn't be re-encoded identically: a bool byte other than 0/1, a string without its NUL, an axis list length that isn't a multiple of 4, or bytes left over after the v2 maps. Encode recomputes the v2 file length at 0x14 and `EndOffset`, and stores both back into the `File`.

- Parse then Encode is byte-identical for all 16 ecuxplot packs. For v2 the original zip is reused while the map block is unchanged.
- KP to JSON to KP: byte-identical for v1. For v2 the map block is re-deflated (Go `compress/flate`, level 9). The inflated block, everything before the zip (except 0x14 and `EndOffset`) and everything after it are identical. Re-deflated zips land within about 0.4% of the original size.
- Editing a string length (which shifts every later offset) and an address re-encodes and parses back with only those changes.

## v2 zip container

Verified 2026-10-07 on all 11 v2 packs in ecuxplot's data:

- One local file entry `intern`, method 8 (deflate), general-purpose flag 2, DOS date and time zero, version 20.
- The stored stream is reproduced exactly by zlib 1.2.x raw deflate at level 9, memLevel 8, window 15. WinOLS 2.24 links zlib 1.2.3 statically.
- The Go implementation uses `compress/flate` (pure Go, decided), whose output differs. A re-encoded v2 file therefore has a different compressed stream, sizes and CRC; the outer zip length int, the file length at 0x14 and `EndOffset` must be updated. WinOLS 2.24 imports such files cleanly (confirmed 2026-10-07 on a re-zipped and a lint-fixed 8D0907551M; before `EndOffset` was updated, the import raised WinOLS's crash-report dialog).

## WinOLS 2.24 exports

Observed 2026-10-07. 8D0907551M.kp was imported into WinOLS 2.24 and exported four times: unchanged, unchanged again, with KFWKSTAB X set to "EEPROM", and with KFLDRQ2 also renamed. A new project with no maps was exported as well. Compared with the original pack:

- Header id 0x124, version string "OLS 5.0 (Windows)". The filename string is the name the file was exported under, usually upper-cased (`W1.KP`).
- The project's `NameUnk00`, `VersionUnk00`, `VersionUnk14` and `MapsUnk00` are identical in every export, including the empty project.
- `File.UnkHeader` differs in two runs, 15 bytes at offset 0x3a and 68 bytes at 0x76 within the block, and two bytes of the first run change on every save. The contents look like uninitialized memory: MSVC heap fill words (`0xbaadf00d`), stack addresses around 0x0019d000 and DLL addresses around 0x73000000. Treat these bytes as don't-care.
- `Project.UnkTrailing` is the same kind of thing: 1214 bytes differ between the original and an export, about 40 between consecutive exports, and most of the changed words look like pointers. A few small values change as well, with unknown meaning.
- Map end address: WinOLS writes start + byte length (one past the last byte) and recomputed it on 13 maps of the original whose stored ends were stale (TVCAMSR: start 90912, end 91094, rewritten as 90914). Stale ends are common in hand-edited packs (4Z7907551AA, 8D0907551K/M, 8D0907558M); lint rule R8 recomputes them (`autocorrect.md`), giving the same values as WinOLS on 8D0907551M.
- Map `RowsUnk00` (int32[2]) changes only on maps opened during the session: KFWKSTAB went from 1, 1 to -1, 0 after its axis edit, the renamed map from 3, 3 to 1, 0, and KFNLLNST from -1, 2 to 0, 2 on import. Most values lie within the map's dimensions, so it is probably editor state such as the last cursor cell (unconfirmed).
- WinOLS probably stores more per-map view state in the undecoded map fields (unconfirmed): map `Unk00` and `ViewScale` (Map record) change with use; its script interface has per-map `ViewMode` (text, 2D, 3D) and `RWin` (hex, bars) properties.
- The `intern` stream of every export is reproduced exactly by zlib at level 9, memLevel 8, as with the ecuxplot packs.
- Exporting twice under the same name didn't replace the file: the second export was appended to the first. The file was saved over RDP drive redirection to a macOS folder, and a TunerPro save to the same folder also left the old file's tail in place, so the redirection may be at fault rather than WinOLS (unconfirmed). The value at 0x14 gave the combined length, `EndOffset` pointed at the first copy's end marker, and the second copy lacked the signature's 4-byte length prefix. `kp.Parse` reads the first copy and keeps the second in `UnkTrailing`.

## Map record

Field order (v2-only fields marked), as declared in `kp.Map`:

- byte `Unk00` (0; 1 on a few dozen maps per v2 pack, and a different set in each WinOLS export of the same project, so editor state); v2: int32 `Unk01` (-1), comment string, byte `CommentUnk00` (0)
- name string (long description)
- int32 organisation, int32 `OrganizationUnk00` (2 in most packs; 0 on every map of 06A906032HS, 3 on most of 4D1907558, a few 0, 1 or 3 elsewhere; unknown, possibly the script's `ViewMode`), int32 value type, int32 width, int32 display base, int32 folder id. The width equals the value type's width in bytes (1, 2, 4) on all 32,315 maps in ecuxplot's packs, so it is the element size (unconfirmed which of the two WinOLS reads).
- id string (short identifier; mapdump strips anything after a space or `?`)
- int32 `IDUnk00` (0), byte `IDUnk04` (0); v2: int32 `IDUnk05` (0 or 1, mostly the same within a pack; unknown)
- int32[4] range (meaning unconfirmed): [low, 0, high, 0], 0 and 255 on almost every map; other pairs (0-256, 0-151, 22-178, 10-90) mostly in hand-edited packs
- int32[8] `RangeUnk00`: two more groups. Elements 0 and 2 are 0 and 65535 on almost every map; on hand-edited maps other pairs, such as 170 and 8149, which is the raw minimum and maximum of KFMIRL's data in 8D0907551H and was copied to other maps with the map properties. Elements 6 and 7 are -1, 0 or 0, 1 (the latter on maps the user edited). Elements 1, 3 and 5 are always 0. Probably display or limit settings; unconfirmed.
- bytes: reciprocal, signed, difference, percent
- int32 columns, int32 rows (patchable)
- int32[2] `RowsUnk00` (probably editor state, see WinOLS 2.24 exports), int32 precision
- value block: description string, units string, float64 factor, float64 offset
- int32 start address, int32 end address (start + byte length as WinOLS writes it; packs can hold stale values), int32 image size (0x100000 in most packs, 0x20000 in 8D0907558E, 0x40000 in 8D0907558M: the image length in all 16 ecuxplot packs, so taken as the image size); v2: int32[2] `ImageSizeUnk00`
- int32 `addr2` (equals the start address on most maps, and on every map in 9 of the 16 packs), int32[2] `Addr2Unk00` (first -1 in every map of 8D0907551F, G, H, K and test8maps, else 0; second always 0), int32 `Addr2ImageSize` (the image length, or 0 in every map of a few packs; name guessed), int32 `Addr2ImageSizeUnk00` (0)
- X axis record, Y axis record
- The tail, 80 bytes in both layouts. Its fields and the values seen in ecuxplot's 32,315 maps:

| Offset | Field | Type | Values |
|---|---|---|---|
| 0x00 | `YUnk00` | int32 | 0; 1 on TNMXH (8D0907551M) and KFLDS.0 (8D0907558M) |
| 0x04 | `YUnk04` | int16 | 0 |
| 0x06 | `StoredCols` | int32 | columns in storage order, see below |
| 0x0A | `StoredRows` | int32 | rows in storage order |
| 0x0E | `StoredRowsUnk00` | int32 | 1 or 3 (3 on most of 8D0907551G and some hand-edited maps); unknown |
| 0x12 | `StoredRowsUnk04` | byte[2] | 1, 1 |
| 0x14 | `StoredRowsUnk06` | float64[2] | 1.0, 1.0 on all but a few dozen maps, all in hand-edited packs |
| 0x24 | `StoredRowsUnk16` | int32 | -1 on all but about 80 maps (0, or values like 44415) |
| 0x28 | `StoredRowsUnk1A` | byte | 1; 0 on 5 maps in 4D1907558 and 8D0907551M |
| 0x29 | `StoredRowsUnk1B` | float64[2] | 300.0, 1.0 on most maps; 0.0, 0.01 on about 1700, many of them 1D maps in 06A906032HS; other pairs in hand-edited packs |
| 0x39 | `ViewScale` | float64 | 0.0 on most maps, see below |
| 0x41 | `ViewScaleUnk00` | int32 | -1 exactly where `ViewScale` is 0.0, else 0 or a value up to about 80000 |
| 0x45 | `ViewOffset` | float64 | 0.0 or -0.0 on most maps, see below |
| 0x4D | `Term2` | byte[3] | 1, 1, 1; the first two must be 1 |

`StoredCols` and `StoredRows` are columns then rows, swapped on "2d Inverse" (column-major) maps: true on every map in the archived packs and the WinOLS 2.24 saves except 11 single values. xdfkit writes them that way.

`ViewScale`, `ViewScaleUnk00` and `ViewOffset` are probably the display's automatic scaling, stored once a map has been shown (unconfirmed). They are non-default on exactly the same maps, mostly in hand-edited packs, and on single values follow the value: `ViewScale` is 2560 divided by a multiple of the value and `ViewOffset` -0.9 times that multiple (8D0907551M: CATR, value 1, 2560 and -0.9; CLAHSH, value 3, 853.33 and -2.7; CWGGPBKV, value 6, 426.67 and -5.4). An earlier guess that they are a second factor and offset doesn't hold. mapdump reads the tail as int32 and int16 chunks that straddle these floats.

## Axis record

- value block (description, units, factor, offset)
- int32 data source (patchable)
- int32 address (patchable; meaningful only for image data sources)
- int32 value type, int32 width, int32 display base; mirror map flag (v2: int32, v1: byte; 0 or 1); v2: 9 zero bytes `MirrorUnk00`. The width equals the value type's width in bytes on all but 2 of the 64,630 axis records in ecuxplot's packs (NMAX and NMAXAL in 06A906032LP: single values with a stray defined u8 axis and width 4), so it is the element size, like the map's.
- byte reciprocal, byte precision
- 3 bytes `PrecisionUnk00` (zero; 0xff 0xff 0xff on the unused slots of 06A906032HS only), byte signed (patchable)
- int32 byte length, then that many bytes of int32 `SignedUnk00`: always zeros. In v1 both axis records of a map hold as many as the map's `StoredRows` on almost every map; in v2 the list is empty, except on the two "free editable" Y axes of LDRXN in 4D1907558 (16 zeros). So possibly the values of a free editable axis (unconfirmed).
- int32 `DataHeader` (was `ListUnk00`): likely the script property `DataHeader`, the number of header bytes before the axis data (surveyed 2026-10-08 over the 12 OEM pack/image pairs): it is 0, 1, 2 or 4, and where it isn't 0 the bytes just before the axis hold the point count in 215 of 217 1-byte headers on byte axes, 544 of 619 2-byte headers on byte axes (the M3.82 [variable id][count] header gives 2 on 462 of 468 such axes in 8D0907558E) and 113 of 130 2-byte headers on word axes. 4-byte headers on word axes match only 28 of 84, maybe [X count][Y count]. Many axes with a header in the image have 0, so it is a setting, not detected.
- int32 `SignatureByte`: likely the script property `SignaturByte` (marker byte before the axis, `0xFFFFFFFF` for none; `winols-script.md`). -1 is the commonest value in every pack. In 8D0907558E (M3.82), 468 of the 477 image axes with another value have that byte two bytes before the axis, which is the variable id of the [variable id][count] header; the match is weaker in 8D0907558M (49 of 311) and in the ME7 packs, so the exact rule is unconfirmed.
- Every map stores both axis records. The organisation says which are used: X for 1D, X and Y for 2D, none for a single value. WinOLS 2.24 goes by the organisation alone, not by what the unused slot holds (confirmed 2026-10-08): NMAXAL in 06A906032LP, a single value whose slots hold eeprom axes, shows no axes, and "16 bit KFZW load axis patch #1" in 8D0907551M, a 1D map with an ordinal Y slot, shows only its ordinal X axis. The unused slots are still stored, often with the mirror flag set (and precision 0xff in v2). The flag is WinOLS 2.24's "mirror map" axis setting, not an "undefined" marker (confirmed 2026-10-08: on in 8D0907551F for KFZW's X axis, off for its Y and for both in 8D0907551G). 8D0907551F sets it on every X axis (724), 8D0907551K on 24 and 8D0907551H on 4, and WinOLS shows those axes. `OLS_LangE.dll` labels it "&Mirror map" in both the X-Axis and Y-Axis dialogs; the script property `bRueckwaerts` ("mirror the data", `winols-script.md`) may be the same setting (unconfirmed). It doesn't mean reversed storage: the mirrored eeprom axes in 8D0907551F and 8D0907551K increase in the image like the others (664 of 704, the rest not monotonic). WinOLS shows a mirrored axis in descending order (8D0907551F KFZW: X stored 512 to 8534, shown from 8534 down; confirmed 2026-10-08), so it is a display reversal along that axis, not an X/Y swap. The map's cells are shown reversed with it, so the map displays correctly (confirmed 2026-10-08). mapdump's CSV and XDF treat the flag as "undefined" and drop those axes' units and scale, and print the scale of unused slots that don't have it, so the archived CSVs of most packs differ from CSVs made from xdfkit's KP output in those columns.

## Enums

| Field | Values |
|---|---|
| Organisation | 2 single value, 3 one-dimensional, 4 two-dimensional, 5 2D inverse |
| Value type | 1 8-bit, 2 16-bit HiLo, 3 16-bit LoHi, 4 32-bit HiLo, 5 32-bit LoHi, 6 float HiLo, 7 float LoHi |
| Axis data source | 0 ordinal ("1, 2, 3, ..."), 1 EEPROM, 2 EEPROM add, 3 EEPROM subtract, 4 free editable and 5 EEPROM backwards (unconfirmed) |
| Display base | 10, 16, 2 |

- 4 and 5 follow the script help's enum order, eDataSrcNone, eRom, eRomAdd, eRomSub, eUserdef, eRomBackwards, on the guess that KP stores the internal enum (decided 2026-10-08, as older mapdump read 4). `OLS_LangE.dll` lists them in UI order instead: "1, 2, 3, ...", "Eprom", "Eprom, add", "Eprom, subtract", "Eprom, backwards", "Free editable". To confirm with a KP saved from a script import that sets `eUserdef` and `eRomBackwards`. No defined axis in the ecuxplot packs uses 4 or 5; 4 appears only on unused axis slots in 4D1907558.
- "2D inverse": mapdump's row-major reading is supported by a smoothness check against the bins (34 maps to 1, 50 ties). XDF Porter marks these column-major; don't copy that.

## Axis datasource "EEPROM, subtract"

The stored values are offsets between breakpoints, counted down from the top of the value range.

- Semantics (confirmed 2026-10-07 in WinOLS 2.24): for raw values raw[0] to raw[n-1], WinOLS displays point i = conversion(256 - (raw[i] + raw[i+1] + ... + raw[n-1])) for 8-bit axes, without wrapping. Each raw value is the gap to the next point, and the last one is the gap to 256. Checked on two axes:
  - FHSA.0 X in 8D0907558E (raw 27, 13, 12, 60, conversion 0.75*X-48) displays 60, 80.25, 90, 99 (raw 144, 171, 184, 196).
  - KFWKSTAB X in 8D0907551M (raw 24, 49, 50, 164, total 287) displays -71.25, -53.25, -16.50, 21.00 (raw -31, -7, 42, 92). The negative first point is how a mislabelled absolute axis shows (below).
  - 16-bit "subtract" axes are presumably counted from 65536 (unconfirmed; none seen).
- Evidence that this is the ECU's meaning, not only WinOLS's (8D0907558E/M, 128 KB Motronic images with axes around 0x8xxx, 273 and 285 axis entries labelled "subtract"): read this way, every "subtract" axis in 8D0907558E increases from 0 or more, and the breakpoints are round numbers in the operating range:
  - `TLAN` (rpm, 40*X): 1840, 2000, 3000, 4000, 4520, 5520, 6000, 6800
  - `KFZW.0` Y (rpm): 600, 800, 1000, 1480, ..., 5520, 6000, 6800
  - `FKHE.0` (%, 0.390625*X): 0, 1.95, 25, 75, 99.61
  - `TADTEVT` (s, 0.5*X): 0, 4, 6.5, 9, 15, 18, 21, 25
  - `FHSA.0` ("Heißstartanhebung", hot-start enrichment, degC): 60, 80.25, 90, 99
  - An earlier reading as a forward running sum (point i = raw[0] + ... + raw[i]) also gives increasing axes, but with odd values (TLAN 160 to 8400 rpm, FHSA.0 -27.75 to 36 degC). It is wrong.
- Layout: the two bytes before these axes are [input variable id][point count], e.g. `[164, 8]` on rpm axes, `[240, n]` on % axes, `[155, n]` on temperature. That's consistent with Motronic M3.x-style axis headers; ME7 axes have only the count byte.
- Pack errors exist in both directions, so the label can't be trusted blindly:
  - 8D0907558E has "subtract" axes labelled plain "EEPROM" (KFFA X: raw 12, 12, 14, 16, 17, 13, ..., 36, which read as "subtract" is a clean 1.0, 1.6, 2.2, ..., 10.5, 11.0 ms/rev).
  - The 27 "subtract" axes in the ME7 packs (8D0907551G/M) are absolute and mislabelled by the pack author: read as "subtract" their totals exceed 256, so they start below zero (KFWKSTAB above). With the datasource set to "EEPROM" (`xdfkit fix`, rule R1), WinOLS 2.24 displays KFWKSTAB as -30, -11.25, -10.5, 75, the plain values.
- Still open (cheap, needs WinOLS): what "EEPROM, add" and "Eprom, backwards" do (neither occurs in any ecuxplot pack). Feature-sweep samples are the backup.
- Implementation:
  - KP reader/writer: the model keeps the datasource exactly as stored (`axis.stored` = absolute / add / subtract / backwards), never "corrected".
  - XDF writer: a plain axis equation can't express this sum over the following cells, so write static LABELs computed from the bin (xdfkit computes them from `-i image`; without an image the axis is written as a plain image axis). TunerPro RT can compute the axis live instead: a helper table at the axis address with per-row equations (last row `(256 - X) * factor + offset`, the others `CELL(ROW()+1;0;FALSE) - X * factor`), linked to the map's axis ("Linked, Scaled", XDF `embedinfo type="3" linkobjid`). TunerPro's author describes this for WinOLS "EEPROM, subtract" axes, with a worked example matching the formula above ([forum](https://tunerpro.net/forum/viewtopic.php?t=3672)). The XDF encoding of per-row equations is unconfirmed. Linked axes are a TunerPro 5 feature (whether the free edition has them is unconfirmed); 5.00.8383 and 5.00.8414 (2014) fixed unresolved axis links on load and a recursion bug that broke exactly this `CELL(ROW()+1; 0; FALSE) - X` pattern, so older builds can show wrong values.
  - Lint: rules R1, R2 and R6 (`autocorrect.md`) flag axes whose label disagrees with the data.
  - mapdump writes "subtract" axes as static labels computed from the bin with the formula above (since 2026-10-07; before, it wrote them as plain axes). TunerPro displays them correctly (FHSA.0 and TLAN in 8D0907558E, confirmed 2026-10-07).
