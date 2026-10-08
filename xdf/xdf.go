// Package xdf writes a model as a TunerPro XDF file (XML, format 1.50), as
// ecuxplot's mapdump did (docs/design.md): categories sorted by name with
// "My maps" first, objects in address order, precision limited to the six
// digits TunerPro shows, and "EEPROM, subtract" axes written as labels read
// from the image, since XDF can't compute them. Non-ASCII text is written as
// character references: TunerPro reads XDF as Windows-1252.
//
// Deliberate differences from mapdump: no "Written <date>" comment, so the
// output is reproducible; no units on axes the model doesn't have; float cells
// get TunerPro's float type flag; and a category repeated by name keeps its
// objects instead of moving them to the first category. The axis mirror flag is
// ignored (docs/design.md).
package xdf

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.nyet.org/xdfkit/model"
)

// maxDigits is the most digits TunerPro shows.
const maxDigits = 6

// EMBEDDEDDATA mmedtypeflags bits. flagFloat is unconfirmed.
const (
	flagSigned = 0x01
	flagLE     = 0x02
	flagFloat  = 0x10000
)

type document struct {
	XMLName xml.Name `xml:"XDFFORMAT"`
	Version string   `xml:"version,attr"`
	Header  header
	Objects []any
}

type header struct {
	XMLName     xml.Name   `xml:"XDFHEADER"`
	FileVersion string     `xml:"fileversion,omitempty"`
	DefTitle    string     `xml:"deftitle,omitempty"`
	Description string     `xml:"description,omitempty"`
	BaseOffset  int        `xml:"baseoffset"`
	Defaults    defaults   `xml:"DEFAULTS"`
	Region      *region    `xml:"REGION"`
	Categories  []category `xml:"CATEGORY"`
}

type defaults struct {
	DataSizeInBits int `xml:"datasizeinbits,attr"`
	SigDigits      int `xml:"sigdigits,attr"`
	OutputType     int `xml:"outputtype,attr"`
	Signed         int `xml:"signed,attr"`
	LSBFirst       int `xml:"lsbfirst,attr"`
	Float          int `xml:"float,attr"`
}

type region struct {
	Type         string `xml:"type,attr"`
	StartAddress string `xml:"startaddress,attr"`
	Size         string `xml:"size,attr"`
	RegionFlags  string `xml:"regionflags,attr"`
	Name         string `xml:"name,attr"`
	Desc         string `xml:"desc,attr"`
}

type category struct {
	Index string `xml:"index,attr"`
	Name  string `xml:"name,attr"`
}

type categoryMem struct {
	Index    int `xml:"index,attr"`
	Category int `xml:"category,attr"`
}

type table struct {
	XMLName     xml.Name    `xml:"XDFTABLE"`
	UniqueID    string      `xml:"uniqueid,attr"`
	Flags       string      `xml:"flags,attr"`
	Title       string      `xml:"title,omitempty"`
	Description string      `xml:"description,omitempty"`
	CategoryMem categoryMem `xml:"CATEGORYMEM"`
	Axes        []axis      `xml:"XDFAXIS"`
}

type constant struct {
	XMLName     xml.Name    `xml:"XDFCONSTANT"`
	UniqueID    string      `xml:"uniqueid,attr"`
	Title       string      `xml:"title,omitempty"`
	Description string      `xml:"description,omitempty"`
	CategoryMem categoryMem `xml:"CATEGORYMEM"`
	Data        embedded    `xml:"EMBEDDEDDATA"`
	Units       string      `xml:"units,omitempty"`
	OutputType  string      `xml:"outputtype,omitempty"`
	DecimalPl   *int        `xml:"decimalpl"`
	link
	Math equation `xml:"MATH"`
}

// axis holds the elements of the three kinds of XDFAXIS (labels, image
// breakpoints, cells) in the order TunerPro writes them; each kind leaves out
// what it doesn't use.
type axis struct {
	ID         string     `xml:"id,attr"`
	UniqueID   string     `xml:"uniqueid,attr,omitempty"`
	Data       embedded   `xml:"EMBEDDEDDATA"`
	Units      string     `xml:"units,omitempty"`
	IndexCount *int       `xml:"indexcount"`
	DecimalPl  *int       `xml:"decimalpl"`
	Min        string     `xml:"min,omitempty"`
	Max        string     `xml:"max,omitempty"`
	OutputType string     `xml:"outputtype,omitempty"`
	EmbedInfo  *embedInfo `xml:"embedinfo"`
	*link
	Labels []label  `xml:"LABEL"`
	Math   equation `xml:"MATH"`
}

type embedded struct {
	TypeFlags       string `xml:"mmedtypeflags,attr,omitempty"`
	Address         string `xml:"mmedaddress,attr,omitempty"`
	ElementSizeBits int    `xml:"mmedelementsizebits,attr"`
	RowCount        int    `xml:"mmedrowcount,attr,omitempty"`
	ColCount        int    `xml:"mmedcolcount,attr,omitempty"`
	MajorStrideBits int    `xml:"mmedmajorstridebits,attr,omitempty"`
}

type embedInfo struct {
	Type int `xml:"type,attr"`
}

// link is the data type, unit type and DA link that TunerPro writes.
type link struct {
	DataType int `xml:"datatype"`
	UnitType int `xml:"unittype"`
	DALink   struct {
		Index int `xml:"index,attr"`
	} `xml:"DALINK"`
}

type label struct {
	Index int    `xml:"index,attr"`
	Value string `xml:"value,attr"`
}

type equation struct {
	Equation string `xml:"equation,attr"`
	Var      struct {
		ID string `xml:"id,attr"`
	} `xml:"VAR"`
}

// Write returns m as XDF. title is the definition title (the file stem).
// Without an image there is no file region, and "subtract" axes are written
// as plain image axes.
func Write(m *model.Model, image []byte, title string) ([]byte, error) {
	d := document{Version: "1.50", Header: header{
		FileVersion: trim(m.Project.Version),
		DefTitle:    trim(title),
		Description: trim(m.Project.Name),
		Defaults:    defaults{DataSizeInBits: 8, SigDigits: 2, OutputType: 1, LSBFirst: 1},
	}}
	if len(image) > 0 {
		d.Header.Region = &region{
			Type: "0xFFFFFFFF", StartAddress: "0x0", Size: hex(len(image)), RegionFlags: "0x0",
			Name: "Binary File", Desc: "This region describes the bin file edited by this XDF",
		}
	}
	names, catIndex := categories(m)
	for i, n := range names {
		d.Header.Categories = append(d.Header.Categories, category{hex(i), trim(n)})
	}
	index := map[*model.Object]int{}
	for i, o := range m.Objects {
		index[o] = i
	}
	for _, o := range byAddress(m) {
		obj, err := object(o, index[o], catIndex, image)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.Key, err)
		}
		d.Objects = append(d.Objects, obj)
	}
	var b bytes.Buffer
	e := xml.NewEncoder(&b)
	e.Indent("", "  ")
	if err := e.Encode(d); err != nil {
		return nil, err
	}
	b.WriteByte('\n')
	return asciiRefs(b.Bytes()), nil
}

// categories returns the category names, unique, sorted with "My maps" first,
// and each category id's index among them. Ids outside 0 to n-1 are left out,
// as mapdump did.
func categories(m *model.Model) ([]string, map[int]int) {
	valid := func(c *model.Category) bool { return c.ID >= 0 && c.ID < len(m.Categories) }
	var names []string
	for _, c := range m.Categories {
		if valid(c) && !slices.Contains(names, c.Name) {
			names = append(names, c.Name)
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "My maps":
			return -1
		case b == "My maps":
			return 1
		}
		return strings.Compare(a, b)
	})
	idx := map[int]int{}
	for _, c := range m.Categories {
		if valid(c) {
			idx[c.ID] = slices.Index(names, c.Name)
		}
	}
	return names, idx
}

// byAddress returns the objects sorted by address, in source order at the
// same address.
func byAddress(m *model.Model) []*model.Object {
	return slices.SortedStableFunc(slices.Values(m.Objects), func(a, b *model.Object) int {
		return cmp.Compare(a.Address, b.Address)
	})
}

// object is the XDFCONSTANT or XDFTABLE of o; index is its position in the
// source, which numbers it.
func object(o *model.Object, index int, catIndex map[int]int, image []byte) (any, error) {
	title, desc := trim(o.Description), trim(o.Comment)
	if o.ID != "" {
		title, _, _ = strings.Cut(o.ID, " ")
		desc = trim(o.Description)
		if c := trim(o.Comment); c != "" {
			desc += "\n" + c
		}
	}
	cat := 0
	if len(o.Categories) > 0 {
		cat = catIndex[o.Categories[0]]
	}
	mem := categoryMem{Category: cat + 1}
	uid := hex(index + 1)
	prec := limitPrecision(o.Value, o.Data.Bits, o.Data.Signed)
	if o.Shape == "value" {
		return &constant{
			UniqueID: uid, Title: trim(title), Description: desc, CategoryMem: mem,
			Data:       cells(o.Data, o.Address),
			Units:      trim(o.Value.Units),
			OutputType: outputType(prec, o.View.Base),
			DecimalPl:  decimalPl(prec),
			Math:       formula(o.Value.Conversion),
		}, nil
	}
	t := &table{UniqueID: uid, Flags: "0x0", Title: trim(title), Description: desc, CategoryMem: mem}
	for _, a := range []struct {
		id   string
		x    *model.Axis
		size int
	}{{"x", o.X, o.Cols}, {"y", o.Y, o.Rows}} {
		x, err := breakpoints(a.id, a.x, a.size, image)
		if err != nil {
			return nil, err
		}
		t.Axes = append(t.Axes, x)
	}
	z := axis{
		ID:        "z",
		Data:      cells(o.Data, o.Address),
		Units:     trim(o.Value.Units),
		DecimalPl: &prec,
		Math:      formula(o.Value.Conversion),
	}
	z.Data.RowCount = o.Rows
	if o.Cols > 1 {
		z.Data.ColCount = o.Cols
	}
	if lo, hi, ok := limits(o.Value.Conversion, o.Data); ok {
		z.Min, z.Max = javaFixed(lo, 6), javaFixed(hi, 6)
	}
	z.OutputType = outputType(prec, o.View.Base)
	if prec != 0 {
		z.OutputType = "1"
	}
	t.Axes = append(t.Axes, z)
	return t, nil
}

// breakpoints is the XDFAXIS of a table's x or y axis: labels for axes not
// read from the image (ordinal, editable, unknown, missing, "backwards") and
// for "subtract" axes, else the breakpoints' image location.
func breakpoints(id string, x *model.Axis, size int, image []byte) (axis, error) {
	a := axis{ID: id, UniqueID: "0x0", IndexCount: &size, link: &link{}}
	ordinal := x == nil || x.Source != "image" || x.Stored == "backwards"
	if !ordinal && (x.Address == nil || x.Data == nil) {
		return a, fmt.Errorf("axis %s: image axis without address or storage", id)
	}
	c, prec, base := model.Conversion{Factor: 1}, 0, 10
	if x != nil {
		a.Units = trim(x.Value.Units)
		c = x.Value.Conversion
		var d model.Data
		if x.Data != nil {
			d = *x.Data
		}
		prec = limitPrecision(x.Value, d.Bits, d.Signed)
		if !ordinal {
			base = x.View.Base
		}
	}
	a.DecimalPl = decimalPl(prec)
	a.OutputType = outputType(prec, base)
	var raw []float64
	if !ordinal && x.Stored == "subtract" {
		raw = subtracted(x, size, image)
	}
	if !ordinal && raw == nil {
		a.Data = cells(*x.Data, *x.Address)
		a.Data.MajorStrideBits = x.Data.Bits
		a.EmbedInfo = &embedInfo{Type: 1}
		a.Math = formula(c)
		return a, nil
	}
	a.Data = embedded{ElementSizeBits: 16, MajorStrideBits: -32}
	for i := range size {
		l := label{Index: i}
		switch {
		case raw != nil:
			l.Value = javaFixed(convert(c, raw[i]), prec)
		case i > 0 || prec != 0:
			l.Value = javaFixed(convert(c, float64(i)), prec)
		}
		a.Labels = append(a.Labels, l)
	}
	a.Math = formula(model.Conversion{Factor: 1})
	return a, nil
}

// cells is the EMBEDDEDDATA locating cells or breakpoints of type d.
func cells(d model.Data, addr model.Addr) embedded {
	f := 0
	if d.Signed {
		f |= flagSigned
	}
	if d.Endian == "little" {
		f |= flagLE
	}
	if d.Float {
		f |= flagFloat
	}
	e := embedded{Address: hex(int(addr)), ElementSizeBits: d.Bits}
	if f != 0 {
		e.TypeFlags = fmt.Sprintf("0x%02X", f)
	}
	return e
}

// outputType is TunerPro's output type for whole numbers (2 integer, 3 hex);
// other values use the header default, 1 (float).
func outputType(prec, base int) string {
	switch {
	case prec != 0:
		return ""
	case base == 16:
		return "3"
	}
	return "2"
}

// decimalPl is the decimal places, left out at the header default of 2.
func decimalPl(prec int) *int {
	if prec == 2 {
		return nil
	}
	return &prec
}

func formula(c model.Conversion) equation {
	e := equation{Equation: "X"}
	if c.Reciprocal || c.Factor != 1 || c.Offset != 0 {
		op := " * X"
		if c.Reciprocal {
			op = " / X"
		}
		e.Equation = plain(c.Factor) + op
		if c.Offset != 0 {
			e.Equation += "+ " + plain(c.Offset)
		}
	}
	e.Var.ID = "X"
	return e
}

func convert(c model.Conversion, raw float64) float64 {
	if c.Reciprocal {
		return c.Factor/raw + c.Offset
	}
	return float64(raw*c.Factor) + c.Offset // no fused multiply-add, as in Java
}

// limitPrecision cuts the decimal places so that the largest value of the
// type has at most maxDigits digits. It reproduces mapdump's arithmetic,
// including Java's 32-bit int shift, so that 32-bit unsigned cells (and axes
// without storage, width 0) are measured at raw 0.
func limitPrecision(v model.Value, bits int, signed bool) int {
	width := bits
	if signed {
		width--
	}
	raw := float64(int32(1)<<(uint(width)&31) - 1)
	if v.Conversion.Reciprocal {
		raw = 1
	}
	intDigits := javaInt(math.Floor(math.Log10(convert(v.Conversion, raw))) + 1)
	prec := v.Precision
	if int64(prec)+int64(intDigits) > maxDigits {
		prec = 0
		if maxDigits > intDigits {
			prec = maxDigits - int(intDigits)
		}
	}
	return prec
}

// javaInt converts as Java's (int) cast: NaN is 0, out of range saturates.
func javaInt(f float64) int32 {
	switch {
	case f != f:
		return 0
	case f >= math.MaxInt32:
		return math.MaxInt32
	case f <= math.MinInt32:
		return math.MinInt32
	}
	return int32(f)
}

// limits returns the converted minimum and maximum over the raw range of an
// integer type.
func limits(c model.Conversion, d model.Data) (lo, hi float64, ok bool) {
	if d.Float || d.Bits == 0 {
		return 0, 0, false
	}
	var rlo, rhi int64
	switch {
	case d.Signed:
		rlo, rhi = -(1 << (d.Bits - 1)), 1<<(d.Bits-1)-1
	case c.Reciprocal:
		rlo, rhi = 1, 1<<d.Bits-1
	default:
		rhi = 1<<d.Bits - 1
	}
	a, b := convert(c, float64(rlo)), convert(c, float64(rhi))
	return math.Min(a, b), math.Max(a, b), true
}

// subtracted returns the points of a "subtract" axis as WinOLS shows them:
// point i is 2^bits minus the sum of raw values i to n-1. Nil if the axis is
// float or doesn't fit in the image.
func subtracted(x *model.Axis, size int, image []byte) []float64 {
	w := x.Data.Bits / 8
	start := int(*x.Address)
	if w == 0 || x.Data.Float || start+size*w > len(image) {
		return nil
	}
	var order binary.ByteOrder = binary.BigEndian
	if x.Data.Endian == "little" {
		order = binary.LittleEndian
	}
	out := make([]float64, size)
	t := int64(1) << (8 * w)
	for i := size - 1; i >= 0; i-- {
		c := image[start+i*w:]
		var u uint64
		switch w {
		case 1:
			u = uint64(c[0])
		case 2:
			u = uint64(order.Uint16(c))
		default:
			u = uint64(order.Uint32(c))
		}
		raw := int64(u)
		if x.Data.Signed {
			raw = int64(u<<(64-8*w)) >> (64 - 8*w)
		}
		t -= raw
		out[i] = float64(t)
	}
	return out
}

// plain spells f with the shortest digits that identify it, without exponent
// or trailing zeros (Java's BigDecimal.valueOf(f).stripTrailingZeros()
// .toPlainString()).
func plain(f float64) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// javaFixed spells f with prec decimal places as Java's String.format("%.Nf")
// does: the shortest digits that identify f, rounded half up.
func javaFixed(f float64, prec int) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(math.Abs(f), 'e', -1, 64), "e")
	digits := []byte(strings.Replace(mant, ".", "", 1))
	e, _ := strconv.Atoi(exp)
	point := e + 1 // digits before the decimal point
	if keep := point + prec; keep < len(digits) {
		up := keep >= 0 && digits[keep] >= '5'
		if keep < 0 {
			keep = 0
		}
		digits = digits[:keep]
		for i := len(digits) - 1; up && i >= 0; i-- {
			if digits[i] == '9' {
				digits[i] = '0'
			} else {
				digits[i]++
				up = false
			}
		}
		if up {
			digits = append([]byte{'1'}, digits...)
			point++
		}
	}
	for point <= 0 {
		digits = append([]byte{'0'}, digits...)
		point++
	}
	for len(digits) < point+prec {
		digits = append(digits, '0')
	}
	s := string(digits[:point])
	if prec > 0 {
		s += "." + string(digits[point:point+prec])
	}
	if math.Signbit(f) {
		s = "-" + s
	}
	return s
}

func hex(v int) string { return fmt.Sprintf("0x%X", v) }

// trim drops leading and trailing spaces and control characters, as Java's
// String.trim does, and turns line ends into LF, as XML readers do.
func trim(s string) string {
	s = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}

// asciiRefs replaces non-ASCII characters in XML text with character
// references.
func asciiRefs(b []byte) []byte {
	var out bytes.Buffer
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r < utf8.RuneSelf {
			out.WriteByte(b[0])
		} else {
			fmt.Fprintf(&out, "&#x%04X;", r)
		}
		b = b[n:]
	}
	return out.Bytes()
}
