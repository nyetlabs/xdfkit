# WinOLS script import format

Source: the WinOLS 2.24 help file (`HelpEn.chm`), topics "Importing with scripts" and "Script language". This is the documented, unlicensed way to create maps in WinOLS from text, and the planned primary route from A2L/DAMOS/XDF into WinOLS. Scripts can only insert maps; they cannot modify existing ones.

## Status

- Not yet tested in WinOLS 2.24 or the demo.
- Open: whether factor/offset use a locale-dependent decimal comma (the help example writes `"1,000000"`), what file extension WinOLS expects, and whether the demo accepts scripts.

## Structure

Drag and drop the script into the project window. One `begin_executable` block per map, each with a unique group name.

```text
//--------------------------------------------------------------------------------------------
// This is a WinOLS Script * Please refer to the WinOLS documentation for details
//--------------------------------------------------------------------------------------------
requires_winols "1.505"
replace_mode absolute
begin_requires
end_requires
begin_executable
  search KF00 eByte 0x00000 0 0% "?"
  begin_insert_map KF00
     set_map_property "Name" "Kennfeld"
     set_map_property "IdName" ""
     set_map_property "Typ" "eZweidim"
     set_map_property "FolderName" "My maps"
     set_map_property "ViewMode" "eViewText"
     set_map_property "RWin" "eBars"
     set_map_property "DataOrg" "eFloatLoHi"
     set_map_property "bKehrwert" "0"
     set_map_property "bVorzeichen" "0"
     set_map_property "bDelta" "0"
     set_map_property "bProzent" "0"
     set_map_property "bOriginal" "0"
     set_map_property "bOriginalWerte" "0"
     set_map_property "Spalten" "16"
     set_map_property "Zeilen" "3"
     set_map_property "Radix" "10"
     set_map_property "Nachkommastellen" "0"
     set_map_property "Kommentar" ""
     set_map_property "Feldwerte.Name" "-"
     set_map_property "Feldwerte.Einheit" "-"
     set_map_property "Feldwerte.Faktor" "1,000000"
     set_map_property "Feldwerte.Offset" "0,000000"
     set_map_property "Feldwerte.StartAddr" "7668"
     set_map_property "StuetzX.Name" "-"
     set_map_property "StuetzX.Einheit" "-"
     set_map_property "StuetzX.Faktor" "1,000000"
     set_map_property "StuetzX.Offset" "0,000000"
     set_map_property "StuetzX.DataSrc" "eRom"
     set_map_property "StuetzX.DataHeader" "0"
     set_map_property "StuetzX.DataAddr" "4096"
     set_map_property "StuetzX.DataOrg" "eFloatLoHi"
     set_map_property "StuetzX.Radix" "10"
     set_map_property "StuetzX.bRueckwaerts" "0"
     set_map_property "StuetzX.bKehrwert" "0"
     set_map_property "StuetzX.bVorzeichen" "0"
     set_map_property "StuetzX.Nachkommastellen" "0"
     set_map_property "StuetzX.SignaturByte" "0xFFFFFFFF"
     set_map_property "StuetzX.SkipBytes" "0"
  end_insert_map
end_executable
```

The `StuetzY.*` properties mirror `StuetzX.*`. The `search ... "?"` line with start address 0 anchors the group so the map lands at the given absolute addresses.

## Properties

Checkboxes take 0 or 1. Addresses are hexdump (file) offsets, written in decimal in the example.

| Property | Meaning | Values |
|---|---|---|
| Name | name of the map or axis | text |
| IdName | internal identifier (normally Damos/ASAP2) | text |
| Typ | map type | eEinzel (single), eEindim (1D), eZweidim (2D), eZweiInv (2D inverted) |
| FolderName | folder (WinOLS 2.08+) | text |
| ViewMode | view mode | eViewText, eView2d, eView3d |
| RWin | right pane in text mode | eRightWinNone, eHex, eBars, eHexBars |
| DataOrg | width and endianness | eByte, eLoHi, eHiLo, eLoHiLoHi, eHiLoHiLo, eFloatLoHi, eFloatHiLo |
| bKehrwert | reciprocal view | 0/1 |
| bVorzeichen | signed | 0/1 |
| bDelta | show difference | 0/1 |
| bProzent | show percentage difference | 0/1 |
| bOriginal | ignore factor and offset | 0/1 |
| bOriginalWerte | show original instead of version values | 0/1 |
| Spalten / Zeilen | columns / rows | integer |
| Radix | number base | 10, 16 |
| Nachkommastellen | decimal places | integer |
| Kommentar | comment | text |
| Einheit | unit (prefixed by Feldwerte./StuetzX./StuetzY.) | text |
| Faktor / Offset | display scaling | number |
| StartAddr / DataAddr | start address of values | integer |
| DataSrc | axis data source | eDataSrcNone, eRom, eRomAdd, eRomSub, eUserdef, eRomBackwards |
| DataHeader | header bytes before the axis, also marked in the hexdump | integer |
| bRueckwaerts | mirror the data | 0/1 |
| SignaturByte | marker byte before the axis, or 0xFFFFFFFF (2.08+) | integer |
| SkipBytes | bytes skipped between axis values (2.08+) | integer |

Prefixes: `Feldwerte.` applies to the map values, `StuetzX.` and `StuetzY.` to the axes.

## Script language notes

- `requires_winols "1.505"`: minimum version; the last language additions were in 1.505.
- `begin_requires` / `end_requires`: applicability checks, no changes.
- `begin_executable` / `end_executable`: commands that modify the project.
- `search Group DataOrg Start Deviation Tolerance "values"`: builds a group of candidate offsets; `?` matches any value.
- `replace`, `unique`, `requires_map`, `requires_hexdump`, `replace_mode`: used by change scripts, not needed for map import.

## Use in this project

xdfkit writes WinOLS scripts as its route from A2L, DAMOS and XDF into WinOLS. Coverage per field: the "WinOLS script" column of `format-matrix.md`. The model's names stay format-neutral; the writer maps them (`bRueckwaerts` from axis `mirror` and `SignaturByte` from axis `signature`, both unconfirmed until a scripted import is saved as KP). Corpus fixes still pending in hand WinOLS projects, and how scripts could carry them: `winols-fixes.md`.
