// Package olscsv reads and writes WinOLS map-list CSV (semicolon-separated,
// CRLF, Latin-1 as needed). See docs/design.md.
package olscsv

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/charmap"

	"go.nyet.org/xdfkit/model"
)

// Magic is the map-list CSV header prefix used by Detect.
const Magic = "Name;IdName;FolderName"

var header = []string{
	"Name", "IdName", "FolderName", "Type", "ViewMode", "RWin", "DataOrg",
	"bReciprocal", "bSigned", "bDelta", "bPercent", "bOriginal", "bOriginalValues",
	"Columns", "Rows", "Radix", "Comment", "Precision", "SkipBytes", "LineSkipBytes",
	"ValueRangeMin", "ValueRangeMax", "ValueRangeLock", "Marker",
	"Fieldvalues.Name", "Fieldvalues.Unit", "Fieldvalues.Factor", "Fieldvalues.Offset", "Fieldvalues.StartAddr",
	"AxisX.Name", "AxisX.IdName", "AxisX.Unit", "AxisX.Factor", "AxisX.Offset", "AxisX.Radix",
	"AxisX.bBackwards", "AxisX.bReciprocal", "AxisX.bSigned", "AxisX.Precision",
	"AxisX.DataSrc", "AxisX.DataHeader", "AxisX.DataAddr", "AxisX.DataOrg", "AxisX.SignatureByte", "AxisX.SkipBytes",
	"AxisY.Name", "AxisY.IdName", "AxisY.Unit", "AxisY.Factor", "AxisY.Offset", "AxisY.Radix",
	"AxisY.bBackwards", "AxisY.bReciprocal", "AxisY.bSigned", "AxisY.Precision",
	"AxisY.DataSrc", "AxisY.DataHeader", "AxisY.DataAddr", "AxisY.DataOrg", "AxisY.SignatureByte", "AxisY.SkipBytes",
}

// Trailing columns added when Write is given an image (scanned cell extremes).
var scanHeader = []string{"Value min", "Value max", "Value min*1", "Value max*1"}

// Read parses a WinOLS CSV into a model. name is stored in provenance.
func Read(data []byte, name string) (*model.Model, error) {
	text, err := decodeLatin1(data)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = ';'
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 1 {
		return nil, fmt.Errorf("olscsv: empty file")
	}
	idx := indexHeader(rows[0])
	if _, ok := idx["Name"]; !ok {
		return nil, fmt.Errorf("olscsv: missing Name column")
	}
	sum := sha256.Sum256(data)
	m := &model.Model{
		Schema: model.SchemaID,
		Provenance: &model.Provenance{
			Format: "csv",
			File:   name,
			SHA256: hex.EncodeToString(sum[:]),
			Origin: "manual",
		},
	}
	folders := map[string]int{}
	for _, row := range rows[1:] {
		o, folder, err := objectFromRow(row, idx)
		if err != nil {
			return nil, err
		}
		if folder != "" {
			id, ok := folders[folder]
			if !ok {
				id = len(m.Categories)
				folders[folder] = id
				m.Categories = append(m.Categories, &model.Category{ID: id, Name: folder})
			}
			o.Categories = []int{id}
		}
		m.Objects = append(m.Objects, o)
	}
	return m, m.Check()
}

// Write emits m as WinOLS map-list CSV (CRLF, semicolon, Latin-1). When image
// is non-nil, appends Value min/max and Value min*1/max*1 from a cell scan.
// ValueRangeMin/Max stay blank (definition limits, not the scan).
func Write(m *model.Model, image []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	w.UseCRLF = true
	h := header
	if image != nil {
		h = append(append([]string(nil), header...), scanHeader...)
	}
	if err := w.Write(h); err != nil {
		return nil, err
	}
	for _, o := range m.Objects {
		row := rowFromObject(o, m)
		if image != nil {
			e, err := model.ScanExtremes(o, image)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", o.Key, err)
			}
			cols := []string{"", "", "", ""}
			if e != nil {
				cols = []string{fmtFloat(e.Min), fmtFloat(e.Max), e.RawMin, e.RawMax}
			}
			row = append(row, cols...)
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	out, err := charmap.ISO8859_1.NewEncoder().Bytes(buf.Bytes())
	if err != nil {
		// Keep UTF-8 when Latin-1 cannot represent a rune.
		return buf.Bytes(), nil
	}
	return out, nil
}

func decodeLatin1(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		data = data[3:]
	}
	// Prefer Latin-1; valid UTF-8 with only ASCII stays identical.
	b, err := charmap.ISO8859_1.NewDecoder().Bytes(data)
	if err != nil {
		return string(data), nil
	}
	return string(b), nil
}

func indexHeader(h []string) map[string]int {
	m := make(map[string]int, len(h))
	for i, s := range h {
		m[strings.TrimSpace(s)] = i
	}
	return m
}

func cell(row []string, idx map[string]int, name string) string {
	i, ok := idx[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func objectFromRow(row []string, idx map[string]int) (*model.Object, string, error) {
	cols, _ := atoi(cell(row, idx, "Columns"))
	rows, _ := atoi(cell(row, idx, "Rows"))
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}
	addr, err := parseAddr(cell(row, idx, "Fieldvalues.StartAddr"))
	if err != nil {
		return nil, "", err
	}
	o := &model.Object{
		ID:          cell(row, idx, "IdName"),
		Description: cell(row, idx, "Name"),
		Comment:     cell(row, idx, "Comment"),
		Address:     model.Addr(addr),
		Rows:        rows,
		Cols:        cols,
		Data:        dataFromOrg(cell(row, idx, "DataOrg"), cell(row, idx, "bSigned") == "1"),
		Value: model.Value{
			Description: cell(row, idx, "Fieldvalues.Name"),
			Units:       dashEmpty(cell(row, idx, "Fieldvalues.Unit")),
			Precision:   atoiDefault(cell(row, idx, "Precision"), 2),
			Conversion: model.Conversion{
				Factor:     atofDefault(cell(row, idx, "Fieldvalues.Factor"), 1),
				Offset:     atofDefault(cell(row, idx, "Fieldvalues.Offset"), 0),
				Reciprocal: cell(row, idx, "bReciprocal") == "1",
			},
		},
		View: model.View{
			Radix:      radixBase(cell(row, idx, "Radix")),
			Difference: cell(row, idx, "bDelta") == "1",
			Percent:    cell(row, idx, "bPercent") == "1",
		},
	}
	if cell(row, idx, "Marker") == "1" {
		o.VisLevel = 1
	}
	switch {
	case rows > 1 && cols > 1:
		o.Shape = "2d"
	case cols > 1 || rows > 1:
		o.Shape = "1d"
	default:
		o.Shape = "value"
	}
	if t := cell(row, idx, "Type"); t == "eZweidimInverse" || t == "e2dInverse" {
		o.Inverse = true
		o.Shape = "2d"
	}
	o.X = axisFromRow(row, idx, "AxisX", cols)
	o.Y = axisFromRow(row, idx, "AxisY", rows)
	folder := cell(row, idx, "FolderName")
	return o, folder, nil
}

func axisFromRow(row []string, idx map[string]int, prefix string, n int) *model.Axis {
	src := cell(row, idx, prefix+".DataSrc")
	a := &model.Axis{
		Source: "ordinal",
		Value: model.Value{
			Description: cell(row, idx, prefix+".Name"),
			Units:       dashEmpty(cell(row, idx, prefix+".Unit")),
			Precision:   atoiDefault(cell(row, idx, prefix+".Precision"), 2),
			Conversion: model.Conversion{
				Factor:     atofDefault(cell(row, idx, prefix+".Factor"), 1),
				Offset:     atofDefault(cell(row, idx, prefix+".Offset"), 0),
				Reciprocal: cell(row, idx, prefix+".bReciprocal") == "1",
			},
		},
		View:   model.View{Radix: radixBase(cell(row, idx, prefix+".Radix"))},
		Header: atoiDefault(cell(row, idx, prefix+".DataHeader"), 0),
	}
	switch src {
	case "eRom", "eEprom", "eEEPROM", "eDataSrcEprom", "eDataSrcRom":
		a.Source = "image"
		a.Stored = "absolute"
	case "eEpromAdd", "eDataSrcEpromAdd":
		a.Source = "image"
		a.Stored = "add"
	case "eEpromSub", "eDataSrcEpromSub", "eEpromSubtract":
		a.Source = "image"
		a.Stored = "subtract"
	case "eEpromBackwards", "eDataSrcEpromBackwards":
		a.Source = "image"
		a.Stored = "backwards"
	case "eFreeEditable", "eDataSrcFreeEditable":
		a.Source = "editable"
	case "", "eDataSrcNone", "None":
		a.Source = "ordinal"
	default:
		if strings.Contains(strings.ToLower(src), "rom") || strings.Contains(strings.ToLower(src), "eprom") {
			a.Source = "image"
			a.Stored = "absolute"
		}
	}
	if a.Source == "image" {
		if addr, err := parseAddr(cell(row, idx, prefix+".DataAddr")); err == nil && addr != 0 {
			ad := model.Addr(addr)
			a.Address = &ad
		}
		d := dataFromOrg(cell(row, idx, prefix+".DataOrg"), cell(row, idx, prefix+".bSigned") == "1")
		a.Data = &d
	}
	if cell(row, idx, prefix+".bBackwards") == "1" {
		a.Stored = "backwards"
	}
	_ = n
	return a
}

func rowFromObject(o *model.Object, m *model.Model) []string {
	folder := ""
	if len(o.Categories) > 0 {
		id := o.Categories[0]
		for _, c := range m.Categories {
			if c != nil && c.ID == id {
				folder = c.Name
				break
			}
		}
	}
	marker := "0"
	if o.VisLevel == 1 {
		marker = "1"
	}
	row := make([]string, len(header))
	set := func(name, v string) {
		for i, h := range header {
			if h == name {
				row[i] = v
				return
			}
		}
	}
	set("Name", o.Description)
	set("IdName", o.ID)
	set("FolderName", folder)
	set("Type", typeFromShape(o))
	set("ViewMode", "eViewText")
	set("RWin", "eBars")
	set("DataOrg", orgFromData(o.Data))
	set("bReciprocal", bool01(o.Value.Conversion.Reciprocal))
	set("bSigned", bool01(o.Data.Signed))
	set("bDelta", bool01(o.View.Difference))
	set("bPercent", bool01(o.View.Percent))
	set("bOriginal", "0")
	set("bOriginalValues", "0")
	set("Columns", strconv.Itoa(o.Cols))
	set("Rows", strconv.Itoa(o.Rows))
	set("Radix", radixFromBase(o.View.Radix))
	set("Comment", o.Comment)
	set("Precision", strconv.Itoa(o.Value.Precision))
	set("SkipBytes", "0")
	set("LineSkipBytes", "0")
	set("ValueRangeLock", "0")
	set("Marker", marker)
	set("Fieldvalues.Name", o.Value.Description)
	set("Fieldvalues.Unit", emptyDash(o.Value.Units))
	set("Fieldvalues.Factor", fmtFloat(o.Value.Conversion.Factor))
	set("Fieldvalues.Offset", fmtFloat(o.Value.Conversion.Offset))
	set("Fieldvalues.StartAddr", fmt.Sprintf("$%X", uint32(o.Address)))
	fillAxis(set, "AxisX", o.X)
	fillAxis(set, "AxisY", o.Y)
	return row
}

func fillAxis(set func(string, string), prefix string, a *model.Axis) {
	if a == nil {
		set(prefix+".DataSrc", "eDataSrcNone")
		set(prefix+".DataAddr", "$0")
		set(prefix+".Factor", "1")
		set(prefix+".Offset", "0")
		return
	}
	set(prefix+".Name", a.Value.Description)
	set(prefix+".Unit", emptyDash(a.Value.Units))
	set(prefix+".Factor", fmtFloat(a.Value.Conversion.Factor))
	set(prefix+".Offset", fmtFloat(a.Value.Conversion.Offset))
	set(prefix+".Radix", radixFromBase(a.View.Radix))
	set(prefix+".bReciprocal", bool01(a.Value.Conversion.Reciprocal))
	set(prefix+".Precision", strconv.Itoa(a.Value.Precision))
	set(prefix+".DataHeader", strconv.Itoa(a.Header))
	set(prefix+".SkipBytes", "0")
	switch a.Source {
	case "image":
		set(prefix+".DataSrc", "eRom")
		if a.Data != nil {
			set(prefix+".DataOrg", orgFromData(*a.Data))
			set(prefix+".bSigned", bool01(a.Data.Signed))
		}
		if a.Address != nil {
			set(prefix+".DataAddr", fmt.Sprintf("$%X", uint32(*a.Address)))
		} else {
			set(prefix+".DataAddr", "$0")
		}
		if a.Stored == "backwards" {
			set(prefix+".bBackwards", "1")
		}
	case "editable":
		set(prefix+".DataSrc", "eFreeEditable")
		set(prefix+".DataAddr", "$0")
	default:
		set(prefix+".DataSrc", "eDataSrcNone")
		set(prefix+".DataAddr", "$0")
	}
}

func dataFromOrg(org string, signed bool) model.Data {
	d := model.Data{Bits: 8, Signed: signed}
	switch org {
	case "eHiLo":
		d.Bits, d.Endian = 16, "big"
	case "eLoHi":
		d.Bits, d.Endian = 16, "little"
	case "eHiLoHiLo", "eFloatHiLo":
		d.Bits, d.Endian, d.Float = 32, "big", strings.Contains(org, "Float")
	case "eLoHiLoHi", "eFloatLoHi":
		d.Bits, d.Endian, d.Float = 32, "little", strings.Contains(org, "Float")
	case "eByte", "eBits", "":
		d.Bits = 8
	default:
		if strings.Contains(org, "Float") {
			d.Bits, d.Float = 32, true
			d.Endian = "little"
		} else if strings.Contains(org, "LoHi") {
			d.Bits, d.Endian = 16, "little"
		} else if strings.Contains(org, "HiLo") {
			d.Bits, d.Endian = 16, "big"
		}
	}
	return d
}

func orgFromData(d model.Data) string {
	switch {
	case d.Float && d.Endian == "big":
		return "eFloatHiLo"
	case d.Float:
		return "eFloatLoHi"
	case d.Bits == 16 && d.Endian == "little":
		return "eLoHi"
	case d.Bits == 16:
		return "eHiLo"
	case d.Bits == 32 && d.Endian == "little":
		return "eLoHiLoHi"
	case d.Bits == 32:
		return "eHiLoHiLo"
	default:
		return "eByte"
	}
}

func typeFromShape(o *model.Object) string {
	if o.Inverse {
		return "eZweidimInverse"
	}
	switch o.Shape {
	case "2d":
		return "eZweidim"
	case "1d":
		return "eEindim"
	default:
		return "eEinzel"
	}
}

func parseAddr(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "$0" || s == "0" {
		return 0, nil
	}
	s = strings.TrimPrefix(s, "$")
	s = strings.TrimPrefix(strings.ToLower(s), "0x")
	v, err := strconv.ParseUint(s, 16, 32)
	return uint32(v), err
}

func atoi(s string) (int, error) {
	if s == "" || s == "-" || s == "None" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func atoiDefault(s string, d int) int {
	v, err := atoi(s)
	if err != nil || s == "" {
		return d
	}
	return v
}

func atofDefault(s string, d float64) float64 {
	if s == "" || s == "-" || s == "None" {
		return d
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return d
	}
	return v
}

func radixBase(s string) int {
	switch s {
	case "16", "eHex", "hex":
		return 16
	case "2", "eBin", "bin":
		return 2
	default:
		return 10
	}
}

func radixFromBase(b int) string {
	switch b {
	case 16:
		return "16"
	case 2:
		return "2"
	default:
		return "10"
	}
}

func bool01(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func dashEmpty(s string) string {
	if s == "-" {
		return ""
	}
	return s
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func fmtFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}
