# KP format ("OLS Map Pack", *.kp)

What is known about WinOLS map packs, as read by ecuxplot's mapdump (`org.nyet.mappack` in [ecuxplot](https://github.com/nyetlabs/ecuxplot)) and the Go port in `../kp/`. All integers are little-endian.

Undecoded fields are named `Unk` plus mapdump's `hN` name: `UnkH9a` in Go, `unkH9a` in the JSON dump. The prefix marks a placeholder: each should get a real name once its meaning is known. The lists below use mapdump's short names (`h9a`). A few named fields are still guesses and say so (`range`, `addr2`, `signature`).

## Versions

| Layout | Header id at 0x10 | Value at 0x14 | Fixed header ends | Maps stored |
|---|---|---|---|---|
| v1 | 0x71, 0x74 | 0 | 0x5c | inline |
| v2 | 0x124, 0x149 | total file length | 0x60 | deflated zip entry `intern` |

- All 11 v2 packs in ecuxplot's data use 0x124; 0x149 is accepted by mapdump but not seen in them.
- Every KP in ecuxplot's data ends with the same 4 bytes `33 88 72 98`, so that is a constant, not a checksum.
- WinOLS 2.24 is `2.24.00` (string in `ols_32on32.exe`); which header id it writes is unconfirmed.
- WinOLS 5 KP files are known not to load in WinOLS 4; whether newer versions read 2.24-era KP is unconfirmed.

## Strings

- int32 length, then that many ISO-8859-15 bytes, then one NUL byte.
- A zero length has no NUL byte.

## File layout

- Signature string ("WinOLS File"), int32 header id, int32 header length.
- Seek to the fixed header end (0x5c or 0x60): filename string, version string.
- Undecoded header, with three int32 `-1` terminators (four in v2), kept raw as `File.UnkHeader`. The exact walk is in `kp.walkHeader` and mapdump's `Parser.parseHeader`. The bytes between the header length and the fixed header end are kept raw as `File.UnkFixed`.
- Project: name string, int32[4] `h`, (v2: 4 bytes `ha`, 0), version string, int32[4] `h1`, (v2: 4 bytes `h1a`, 0), int32 `h77`, (v2: one byte `h77a`).
- Maps:
  - v1: int32 count, then the map records.
  - v2: int32 zip length, then that many bytes of zip archive, then the folder table follows directly. (mapdump's `Project.java` rewinds to `start + zsize` and reads one int, which nets out to no extra field.) The zip holds one entry `intern`; its inflated content is one byte, int32 count, then the map records.
- int32[3] `h2`.
- Folders: int32 count, then per folder int32 id, int32 `h`, name string, 2 bytes `h1`, int32 `h2`, (v2: 15 bytes `h3`). mapdump sorts folders by name, drops duplicate names and renumbers map folder ids; the Go reader keeps them raw.
- Trailing bytes (not decoded; kept raw as `Project.UnkTrailing`).

## Round trip

`kp.Parse` and `File.Encode` share one layout walk (`kp/codec.go`), and every byte is either a field or a raw `Hex` block. The record layouts are the Go structs themselves: fields are coded in declaration order by reflection, with `kp:"..."` struct tags for layout-only fields (`v1`, `v2`), raw lengths (`len=N`), count- or byte-length-prefixed lists (`count`, `bytes`), the trailing bytes (`rest`) and hand-coded fields (`hook`: the fixed header, the header walk and the v2 map block). The tag grammar is documented at the top of `codec.go`.

`File.Offsets(paths...)` returns where fields are stored, by JSON path (`project.maps[3].x.dataSource`), in one walk: in the file, or for v2 maps in the inflated map block. `File.Offset(path)` is the single-path form. The patch-mode writer uses it to find fields to patch. Decoding fails on anything that couldn't be re-encoded identically: a bool byte other than 0/1, a string without its NUL, an axis list length that isn't a multiple of 4, or bytes left over after the v2 maps. Encode recomputes the v2 file length at 0x14.

- Parse then Encode is byte-identical for all 16 ecuxplot packs. For v2 the original zip is reused while the map block is unchanged.
- KP to JSON to KP: byte-identical for v1. For v2 the map block is re-deflated (Go `compress/flate`, level 9). The inflated block, everything before the zip (except 0x14) and everything after it are identical. Re-deflated zips land within about 0.4% of the original size.
- Editing a string length (which shifts every later offset) and an address re-encodes and parses back with only those changes.

## v2 zip container

Verified 2026-10-07 on all 11 v2 packs in ecuxplot's data:

- One local file entry `intern`, method 8 (deflate), general-purpose flag 2, DOS date and time zero, version 20.
- The stored stream is reproduced exactly by zlib 1.2.x raw deflate at level 9, memLevel 8, window 15. WinOLS 2.24 links zlib 1.2.3 statically.
- The Go implementation uses `compress/flate` (pure Go, decided), whose output differs. A patched v2 file therefore has a different compressed stream, sizes and CRC; the outer zip length int and the file length at 0x14 must be updated. A WinOLS load test must confirm that such files open.

## Map record

Field order (v2-only fields marked), as declared in `kp.Map`:

- byte `h0`; v2: int32 `h0a` (-1), comment string, byte `h0b`
- name string (long description)
- int32 organisation, int32 `h`, int32 value type, int32 `ha`, int32 display base, int32 folder id
- id string (short identifier; mapdump strips anything after a space or `?`)
- int32 `h1`, byte `h1a`; v2: int32 `h1b`
- int32[4] range (meaning unconfirmed; 0-255 on every v1 map, varies in v2)
- int32[8] `h2`
- bytes: reciprocal, signed, difference, percent
- int32 columns, int32 rows (patchable)
- int32[2] `h3`, int32 precision
- value block: description string, units string, float64 factor, float64 offset
- int32 start address, int32 end address, int32 `h4`; v2: int32[2] `h4a`
- int32 `addr2`, int32[2] `h5`, int32 `h6`, int32 `h7`
- X axis record, Y axis record
- int32 `h8`, int16 `h8a`, int32[5] `h9`, int16[7] `h9a`, int32 `h9b`, byte `h9c`, int32[6] `h10` (holds doubles such as 300.0, 1.0, -0.9), int32[2] `h11`
- 3 bytes `term2`; the first two must be 1

## Axis record

- value block (description, units, factor, offset)
- int32 data source (patchable)
- int32 address (patchable; meaningful only for image data sources)
- int32 value type, int32 `h1`, int32 display base; v2: int32[3] `h1a`
- byte `h2`, byte reciprocal, byte precision
- 3 bytes `h3`, byte signed (patchable)
- int32 `h4` byte length, then that many bytes of int32 `h4` (sized 4 bytes per point on v1 free-editable axes)
- int32 `h5`, int32 signature (candidate for the help file's `SignaturByte`)
- Undefined slots (second axis of a 1D map, both axes of a single value) are stored anyway: v2 marks them with `h1a[0] = 1` (precision 0xff), v1 with `h2 = 1`.

## Enums

| Field | Values |
|---|---|
| Organisation | 2 single value, 3 one-dimensional, 4 two-dimensional, 5 2D inverse |
| Value type | 1 8-bit, 2 16-bit HiLo, 3 16-bit LoHi, 4 32-bit HiLo, 5 32-bit LoHi, 6 float HiLo, 7 float LoHi |
| Axis data source | 0 ordinal ("1, 2, 3, ..."), 1 EEPROM, 2 EEPROM add, 3 EEPROM subtract, 4 and 5 unconfirmed |
| Display base | 10, 16, 2 |

- `OLS_LangE.dll` lists the data sources in UI order as "1, 2, 3, ...", "Eprom", "Eprom, add", "Eprom, subtract", "Eprom, backwards", "Free editable". mapdump calls 4 "Free editable"; it is probably "Eprom, backwards", with 5 "Free editable". No ecuxplot pack uses 4 or 5.
- The script help names them eDataSrcNone, eRom, eRomAdd, eRomSub, eUserdef, eRomBackwards (in a different order, so not proof of numbering).
- "2D inverse": mapdump's row-major reading is supported by a smoothness check against the bins (34 maps to 1, 50 ties). XDF Porter marks these column-major; don't copy that.

## Axis datasource "EEPROM, subtract"

Resolved from the bins; WinOLS confirmation pending.

- Semantics, resolved from the bins: a running sum. Displayed axis point i = conversion(raw[0] + raw[1] + ... + raw[i]): the first stored value is the starting point and each later value is an offset from the previous point. The sum is not wrapped to 8 bits (`FKHE.0` totals 256, which displays as exactly 100%).
- Evidence (8D0907558E/M, 128 KB Motronic images with axes around 0x8xxx): 273 and 285 axis entries labelled "subtract"; most are non-monotonic read plainly but monotonic as running sums, and the physical values only make sense summed:
  - `TLAN` (rpm, 40*X): summed 160, 1160, 2160, 2680, 3680, 4160, 4960, 8400; plain 160, 1000, 1000, 520, ...
  - `FKHE.0` (%, 0.390625*X): summed 1.95, 25, 75, 99.61, 100; plain 1.95, 23.05, 50, 24.61, 0.39
  - `TADTEVT` (s, 0.5*X): summed 4, 6.5, 9, 15, 18, 21, 25, 128
  - The last offset is usually large, putting the final breakpoint well above the operating range (8400-10000 rpm), presumably so the ECU's subtract-until-negative lookup always terminates. That would explain WinOLS calling it "subtract".
- Layout: the two bytes before these axes are [input variable id][point count], e.g. `[164, 8]` on rpm axes, `[240, n]` on % axes, `[155, n]` on temperature. That's consistent with Motronic M3.x-style axis headers; ME7 axes have only the count byte.
- Pack errors exist in both directions, so the label can't be trusted blindly:
  - 8D0907558E has delta-encoded axes labelled plain "EEPROM" (e.g. KFFA X: 12, 12, 14, 16, 17, 13, ... sums to a clean increasing axis). Across all ecuxplot packs, 275 plain-labelled axes are non-monotonic plain but monotonic summed (not all are delta axes; some are padding or garbage).
  - The 27 "subtract" axes in the ME7 packs (8D0907551G/M) are absolute: summing them gives implausible values (KFWKSTAB up to 167.25 degC, KFTADMS up to 8600 rpm). They are probably mislabelled by the pack author; WinOLS would then display the summed (wrong) values.
- Still open (cheap, needs WinOLS 2.24): what "EEPROM, add" and "Eprom, backwards" do (neither occurs in any ecuxplot pack), and confirmation that WinOLS applies the running sum for "subtract". Opening KFWKSTAB from 8D0907551M.kp should show -30, 6.75, 44.25, 167.25 if it does, or -30, -11.25, -10.5, 75 if it doesn't. Feature-sweep samples are the backup.
- Implementation:
  - KP reader/writer: the model keeps the datasource exactly as stored (`axis.deposit` = absolute / add / subtract), never "corrected".
  - XDF writer: an XDF equation can't express a running sum, so write static LABELs computed from the bin (mapdump already requires `-i bin` for XDF output). Report a loss when no bin is given.
  - Lint (warning only): flag axes whose label disagrees with the data, meaning a plain axis that is monotonic only when summed, or a "subtract" axis that is monotonic plain and implausible summed.
  - mapdump currently treats "subtract" as a plain axis, so its XDFs for 8D0907558E/M are wrong on every summed axis.
