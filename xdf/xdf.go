// Package xdf writes a model as a TunerPro XDF file (XML, format 1.80;
// docs/design.md): categories sorted by name with "My maps" first, objects in
// address order, precision limited to the six digits TunerPro shows, and
// "EEPROM, subtract" axes written as a live helper table (docs/kp-format.md).
// Non-ASCII text is written as character references: TunerPro reads XDF as
// Windows-1252. Float cells get TunerPro's float type flag; float table/axis
// cells with a non-identity conversion fail the write. A category repeated by
// name keeps its objects. An image axis whose axis points are another curve
// links to it. The axis mirror flag is ignored.
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

// minOffset is the smallest conversion offset written: smaller ones can't
// visibly change a shown value, and in plain notation they can exceed the 255
// bytes TunerPro reads from an attribute, which crashes it on load.
const minOffset = 0.5e-6

// EMBEDDEDDATA mmedtypeflags bits. flagFloat is IEEE float32 (TunerPro
// 6.00.0149.00 load and save; 64-bit size is not double).
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
	VisLevel    string      `xml:"vislevel,attr"`
	Flags       string      `xml:"flags,attr"`
	Title       string      `xml:"title,omitempty"`
	Description string      `xml:"description,omitempty"`
	CategoryMem categoryMem `xml:"CATEGORYMEM"`
	Axes        []axis      `xml:"XDFAXIS"`
}

type constant struct {
	XMLName     xml.Name    `xml:"XDFCONSTANT"`
	UniqueID    string      `xml:"uniqueid,attr"`
	VisLevel    string      `xml:"vislevel,attr"`
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
// axis points, cells) in the order TunerPro writes them; each kind leaves out
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
	Labels []label    `xml:"LABEL"`
	Math   []equation `xml:"MATH"`
}

type embedded struct {
	TypeFlags       string `xml:"mmedtypeflags,attr,omitempty"`
	Address         string `xml:"mmedaddress,attr,omitempty"`
	ElementSizeBits int    `xml:"mmedelementsizebits,attr"`
	RowCount        int    `xml:"mmedrowcount,attr,omitempty"`
	ColCount        int    `xml:"mmedcolcount,attr,omitempty"`
	MajorStrideBits int    `xml:"mmedmajorstridebits,attr,omitempty"`
}

// embedInfo is type 1 for axis points read from the image, or type 3 with the
// uniqueid of the object holding them (a linked axis).
type embedInfo struct {
	Type   int    `xml:"type,attr"`
	LinkID string `xml:"linkobjid,attr,omitempty"`
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
	Row      string `xml:"row,attr,omitempty"`
	Var      struct {
		ID string `xml:"id,attr"`
	} `xml:"VAR"`
}

// livePrefix starts a helper table's description. The reader leaves those
// tables out of the model; the metadata file keeps the axis.
const livePrefix = "xdfkit-live:"

// Write returns m as XDF. title is the definition title (the file stem).
// "EEPROM, subtract" axes are live helper tables. Without an image there is
// no file region.
func Write(m *model.Model, image []byte, title string) ([]byte, error) {
	return write(m, image, title, false)
}

// WriteLabels is Write with "subtract" axes as static labels read from the
// image (TunerPro before 5.00.8414). Without an image those axes are plain
// image axes.
func WriteLabels(m *model.Model, image []byte, title string) ([]byte, error) {
	return write(m, image, title, true)
}

func write(m *model.Model, image []byte, title string, labels bool) ([]byte, error) {
	desc := trim(m.Project.Name)
	if p := m.Provenance; p != nil && p.Subset != nil {
		desc = strings.TrimPrefix(desc+"\n"+subsetPrefix+" "+p.Subset.Kind+" sha256:"+p.Subset.Table, "\n")
	}
	d := document{Version: "1.80", Header: header{
		FileVersion: trim(m.Project.Version),
		DefTitle:    trim(title),
		Description: desc,
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
	at := map[model.Addr][]*model.Object{}
	exp := 0
	for i, o := range m.Objects {
		index[o] = i
		at[o.Address] = append(at[o.Address], o)
		exp += len(expansions(o))
	}
	hnext := len(m.Objects) + exp
	helpers := map[string]*table{}
	var helperOrder []string
	link := func(self *model.Object, x *model.Axis, n int) string {
		if !labels && liveOK(x, n) {
			c := x.Value.Conversion
			k := fmt.Sprintf("%d/%d/%s/%d/%s/%s", *x.Address, x.Data.Bits, x.Data.Endian, n, plain(c.Factor), plain(c.Offset))
			if t, ok := helpers[k]; ok {
				return t.UniqueID
			}
			hnext++
			id := hex(hnext)
			cat := 0
			if len(self.Categories) > 0 {
				cat = catIndex[self.Categories[0]]
			}
			t := liveTable(id, x, n, categoryMem{Category: cat + 1})
			helpers[k] = t
			helperOrder = append(helperOrder, k)
			return id
		}
		if x == nil || x.Address == nil {
			return ""
		}
		for _, t := range at[*x.Address] {
			if t != self && linkable(t, x, n) {
				return hex(index[t] + 1)
			}
		}
		return ""
	}
	next := len(m.Objects)
	for _, o := range byAddress(m) {
		obj, err := object(o, index[o], catIndex, link, image, labels)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", o.Key, err)
		}
		d.Objects = append(d.Objects, obj)
		for _, c := range expansions(o) {
			obj, err = object(c, next, catIndex, link, image, labels)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", o.Key, err)
			}
			tagFold(obj, hex(index[o]+1))
			d.Objects = append(d.Objects, obj)
			next++
		}
	}
	for _, k := range helperOrder {
		d.Objects = append(d.Objects, helpers[k])
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
// and each category id's index among them. Ids outside 0 to n-1 are left out.
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

// expansions are the extra tables of o: one per category after the first, and
// one per alternate per alternate category. A map with one category and no
// alternate has none, so archive packs are unchanged.
func expansions(o *model.Object) []*model.Object {
	if len(o.Categories) < 2 && len(o.Alternates) == 0 {
		return nil
	}
	var out []*model.Object
	for _, id := range o.Categories[1:] {
		out = append(out, foldCopy(o, []int{id}, nil))
	}
	for i := range o.Alternates {
		alt := &o.Alternates[i]
		if len(alt.Categories) == 0 {
			out = append(out, foldCopy(o, nil, alt))
			continue
		}
		for _, id := range alt.Categories {
			out = append(out, foldCopy(o, []int{id}, alt))
		}
	}
	return out
}

// foldCopy is o shown as one extra table: cat is its only category, and alt,
// when set, replaces the display. Address and storage stay on o.
func foldCopy(o *model.Object, cat []int, alt *model.Alternate) *model.Object {
	c := *o
	c.Categories = append([]int{}, cat...)
	c.Alternates = nil
	if o.X != nil {
		x := *o.X
		c.X = &x
	}
	if o.Y != nil {
		y := *o.Y
		c.Y = &y
	}
	if alt == nil {
		return &c
	}
	c.ID, c.Description, c.Comment = alt.ID, alt.Description, ""
	c.Value, c.View = alt.Value, alt.View
	if alt.X != nil && c.X != nil {
		c.X.Value = *alt.X
	}
	if alt.Y != nil && c.Y != nil {
		c.Y.Value = *alt.Y
	}
	return &c
}

// tagFold makes the first line of an extra table's description name the
// primary table, so the reader can fold it back.
func tagFold(obj any, primary string) {
	line := foldPrefix + " " + primary
	switch o := obj.(type) {
	case *table:
		o.Description = prefixLine(line, o.Description)
	case *constant:
		o.Description = prefixLine(line, o.Description)
	}
}

func prefixLine(line, rest string) string {
	if rest == "" {
		return line
	}
	return line + "\n" + rest
}

// byAddress returns the objects sorted by address, in source order at the
// same address.
func byAddress(m *model.Model) []*model.Object {
	return slices.SortedStableFunc(slices.Values(m.Objects), func(a, b *model.Object) int {
		return cmp.Compare(a.Address, b.Address)
	})
}

// object is the XDFCONSTANT or XDFTABLE of o; index is its position in the
// source, which numbers it. link returns the uniqueid of the curve an image
// axis of n points links to, or "".
func object(o *model.Object, index int, catIndex map[int]int, link func(*model.Object, *model.Axis, int) string, image []byte, labels bool) (any, error) {
	title, desc := text(o)
	cat := 0
	if len(o.Categories) > 0 {
		cat = catIndex[o.Categories[0]]
	}
	mem := categoryMem{Category: cat + 1}
	uid := hex(index + 1)
	vl := xdfVisLevel(o)
	prec := limitPrecision(o.Value, o.Data.Bits, o.Data.Signed)
	if err := floatDataOK(o.Data); err != nil {
		return nil, err
	}
	if o.Shape == "value" {
		return &constant{
			UniqueID: uid, VisLevel: vl, Title: title, Description: desc, CategoryMem: mem,
			Data:       cells(o.Data, o.Address),
			Units:      trim(o.Value.Units),
			OutputType: outputType(prec, o.View.Base, shown(o.Value.Conversion, o.Data, o.Address, 1, image)),
			DecimalPl:  decimalPl(prec),
			Math:       formula(o.Value.Conversion),
		}, nil
	}
	// Float table cells only accept equation X in TunerPro; a factor/offset
	// draws its round-trip warning. Constants may use a non-identity equation.
	if o.Data.Float {
		if err := floatEqOK(o.Value.Conversion); err != nil {
			return nil, err
		}
	}
	t := &table{UniqueID: uid, VisLevel: vl, Flags: "0x0", Title: title, Description: desc, CategoryMem: mem}
	for _, a := range []struct {
		id   string
		x    *model.Axis
		size int
	}{{"x", o.X, o.Cols}, {"y", o.Y, o.Rows}} {
		x, err := axisPoints(a.id, a.x, a.size, image, labels)
		if err != nil {
			return nil, err
		}
		if x.EmbedInfo != nil {
			if id := link(o, a.x, a.size); id != "" {
				x.EmbedInfo = &embedInfo{Type: 3, LinkID: id}
			}
		}
		t.Axes = append(t.Axes, x)
	}
	z := axis{
		ID:        "z",
		Data:      cells(o.Data, o.Address),
		Units:     trim(o.Value.Units),
		DecimalPl: &prec,
		Math:      eqs(o.Value.Conversion),
	}
	z.Data.RowCount = o.Rows
	if o.Cols > 1 {
		z.Data.ColCount = o.Cols
	}
	applyRange(&z, o.Value.Min, o.Value.Max)
	z.OutputType = outputType(prec, o.View.Base, shown(o.Value.Conversion, o.Data, o.Address, o.Rows*o.Cols, image))
	if prec != 0 {
		z.OutputType = "1"
	}
	t.Axes = append(t.Axes, z)
	return t, nil
}

// linkable says whether t, an object at the address of image axis x, holds
// exactly x's n axis points: a curve of n cells stored and converted as x is.
// Factors match to 1e-4 relative, the rounding KP factors carry; offsets to
// minOffset, below which they aren't written.
func linkable(t *model.Object, x *model.Axis, n int) bool {
	c, d := t.Value.Conversion, x.Value.Conversion
	return t.Shape == "1d" && t.Rows*t.Cols == n && x.Data != nil && t.Data == *x.Data &&
		c.Reciprocal == d.Reciprocal &&
		math.Abs(c.Factor-d.Factor) <= 1e-4*max(math.Abs(c.Factor), math.Abs(d.Factor)) &&
		math.Abs(c.Offset-d.Offset) < minOffset
}

// text is the XDF title and description of o: the first word of its id and
// its description plus comment, or without an id, its description and comment.
func text(o *model.Object) (title, desc string) {
	if o.ID == "" {
		return trim(o.Description), trim(o.Comment)
	}
	title, _, _ = strings.Cut(o.ID, " ")
	desc = trim(o.Description)
	if c := trim(o.Comment); c != "" {
		desc += "\n" + c
	}
	return trim(title), desc
}

// labels reports whether the writer gives axis x labels instead of an image
// location (see axisPoints; "subtract" axes get labels only with an image).
func labels(x *model.Axis) bool {
	return x == nil || x.Source != "image" || x.Stored == "backwards"
}

// axisPoints is the XDFAXIS of a table's x or y axis: labels for axes not
// read from the image (ordinal, editable, unknown, missing, "backwards") and
// for "subtract" axes, else the axis points' image location.
func axisPoints(id string, x *model.Axis, size int, image []byte, static bool) (axis, error) {
	a := axis{ID: id, UniqueID: "0x0", IndexCount: &size, link: &link{}}
	ordinal := labels(x)
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
	if !ordinal && !static && liveOK(x, size) {
		a.Data = embedded{ElementSizeBits: 16, MajorStrideBits: -32}
		a.EmbedInfo = &embedInfo{Type: 3}
		a.Math = eqs(model.Conversion{Factor: 1})
		return a, nil
	}
	var raw []float64
	if !ordinal && x.Stored == "subtract" {
		raw = subtracted(x, size, image)
	}
	if !ordinal && raw == nil {
		d := *x.Data
		if err := floatDataOK(d); err != nil {
			return a, fmt.Errorf("axis %s: %w", id, err)
		}
		if d.Float {
			if err := floatEqOK(c); err != nil {
				return a, fmt.Errorf("axis %s: %w", id, err)
			}
		}
		a.OutputType = outputType(prec, base, shown(c, d, *x.Address, size, image))
		a.Data = cells(d, *x.Address)
		a.Data.MajorStrideBits = d.Bits
		a.EmbedInfo = &embedInfo{Type: 1}
		applyRange(&a, x.Value.Min, x.Value.Max)
		a.Math = eqs(c)
		return a, nil
	}
	a.Data = embedded{ElementSizeBits: 16, MajorStrideBits: -32}
	vals := make([]float64, size)
	for i := range size {
		switch {
		case raw != nil:
			vals[i] = c.Apply(raw[i])
		case x != nil && i < len(x.Values):
			vals[i] = x.Values[i] // already display units
		default:
			vals[i] = c.Apply(float64(i))
		}
		a.Labels = append(a.Labels, label{Index: i, Value: javaFixed(vals[i], prec)})
	}
	a.OutputType = outputType(prec, base, vals)
	a.Math = eqs(model.Conversion{Factor: 1})
	return a, nil
}

func eqs(c model.Conversion) []equation { return []equation{formula(c)} }

// liveOK reports an unsigned integer "subtract" axis the live helper can show.
func liveOK(x *model.Axis, n int) bool {
	if x == nil || x.Stored != "subtract" || x.Address == nil || x.Data == nil || n < 1 {
		return false
	}
	d := x.Data
	return !d.Float && !d.Signed && (d.Bits == 8 || d.Bits == 16 || d.Bits == 32) && !x.Value.Conversion.Reciprocal
}

// liveTable is the helper at the axis address: one column, n rows, each row
// an equation. TunerPro 6.00.0149.00 shows that pattern (docs/kp-format.md).
func liveTable(id string, x *model.Axis, n int, cat categoryMem) *table {
	one := 1
	stub := embedded{ElementSizeBits: 16, MajorStrideBits: -32}
	ident := eqs(model.Conversion{Factor: 1})
	z := axis{
		ID: "z", Data: cells(*x.Data, *x.Address), Units: trim(x.Value.Units),
		DecimalPl: decimalPl(limitPrecision(x.Value, x.Data.Bits, false)),
		Math:      liveEquations(x, n),
	}
	z.Data.RowCount = n
	z.Data.ColCount = 1
	return &table{
		UniqueID: id, VisLevel: "2", Flags: "0x0",
		Title: hex(int(*x.Address)), Description: livePrefix, CategoryMem: cat,
		Axes: []axis{
			{ID: "x", UniqueID: "0x0", Data: stub, IndexCount: &one, link: &link{}, Math: ident},
			{ID: "y", UniqueID: "0x0", Data: stub, IndexCount: &n, link: &link{}, Math: ident},
			z,
		},
	}
}

func liveEquations(x *model.Axis, n int) []equation {
	span := int64(1) << x.Data.Bits
	f, off := x.Value.Conversion.Factor, x.Value.Conversion.Offset
	term := plain(f) + " * X"
	if f == 1 {
		term = "X"
	}
	cell := "CELL(ROW()+1;0;FALSE) - " + term
	last := fmt.Sprintf("(%d - X) * %s", span, plain(f))
	switch {
	case math.Abs(off) < minOffset:
	case off < 0:
		last += " - " + plain(-off)
	default:
		last += "+ " + plain(off)
	}
	out := make([]equation, n)
	for i := range n {
		eq := cell
		if i == n-1 {
			eq = last
		}
		out[i] = equation{Equation: eq, Row: strconv.Itoa(i + 1)}
		out[i].Var.ID = "X"
	}
	return out
}

// cells is the EMBEDDEDDATA locating cells or axis points of type d.
func cells(d model.Data, addr model.Addr) embedded {
	f := 0
	if d.Signed {
		f |= flagSigned
	}
	// Wider cells default to little when endian is unset (C167 / ME7). Always
	// write mmedtypeflags for them so an explicit big (0x00) survives a read
	// that treats a missing flag as the header default.
	if d.Bits > 8 && d.Endian != "big" {
		f |= flagLE
	}
	if d.Float {
		f |= flagFloat
	}
	e := embedded{Address: hex(int(addr)), ElementSizeBits: d.Bits}
	if f != 0 || d.Bits > 8 {
		e.TypeFlags = fmt.Sprintf("0x%02X", f)
	}
	return e
}

// outputType is TunerPro's output type. 3 is hex. 2 is integer, and TunerPro
// shows a 0 in that format as a blank cell, so it is used only when every
// value is a positive whole number. Anything else uses the header default, 1
// (float). Precision other than 0 already asks for a decimal display.
func outputType(prec, base int, vals []float64) string {
	switch {
	case prec != 0:
		return ""
	case base == 16:
		return "3"
	case positiveInts(vals):
		return "2"
	}
	return ""
}

// positiveInts reports that every value is a whole number greater than 0.
func positiveInts(vals []float64) bool {
	if len(vals) == 0 {
		return false
	}
	for _, v := range vals {
		if v != math.Trunc(v) || v <= 0 || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// shown is the converted value of n cells at addr, or nil when the image
// does not hold them.
func shown(c model.Conversion, d model.Data, addr model.Addr, n int, image []byte) []float64 {
	w := d.Bits / 8
	start := int(addr)
	if d.Float || w == 0 || n < 1 || image == nil || start < 0 || start+n*w > len(image) {
		return nil
	}
	order := byteOrder(d)
	out := make([]float64, n)
	for i := range n {
		p := image[start+i*w:]
		out[i] = c.Apply(float64(rawInt(p, order, w, d.Signed)))
	}
	return out
}

// byteOrder is the cell byte order; unset endian is little (C167 / ME7).
func byteOrder(d model.Data) binary.ByteOrder {
	if d.Endian == "big" {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

// applyRange writes min and max when the model has both, ordered low to
// high. A missing pair is left out. The conversion of the raw endpoints is
// not a known range.
func applyRange(a *axis, vmin, vmax *float64) {
	if vmin == nil || vmax == nil {
		return
	}
	lo, hi := *vmin, *vmax
	if math.IsNaN(lo+hi) || math.IsInf(lo, 0) || math.IsInf(hi, 0) {
		return
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	a.Min, a.Max = javaFixed(lo, 6), javaFixed(hi, 6)
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
	offset := math.Abs(c.Offset) >= minOffset
	if c.Reciprocal || c.Factor != 1 || offset {
		op := " * X"
		if c.Reciprocal {
			op = " / X"
		}
		e.Equation = plain(c.Factor) + op
		if offset {
			e.Equation += "+ " + plain(c.Offset)
		}
	}
	e.Var.ID = "X"
	return e
}

// identityEq reports a conversion that writes as equation X.
func identityEq(c model.Conversion) bool {
	return !c.Reciprocal && c.Factor == 1 && math.Abs(c.Offset) < minOffset
}

// floatDataOK rejects float cells that are not 32-bit. TunerPro's float flag
// is single precision; size 64 still reads float32 from the low four bytes.
func floatDataOK(d model.Data) error {
	if !d.Float || d.Bits == 32 {
		return nil
	}
	return fmt.Errorf("float cells must be 32 bits (source data bits=%d)", d.Bits)
}

// floatEqOK rejects a non-identity conversion on float table/axis cells.
// TunerPro warns that such equations may not reverse floating-point data.
func floatEqOK(c model.Conversion) error {
	if identityEq(c) {
		return nil
	}
	return fmt.Errorf("float table/axis cells require equation X (source conversion factor=%g offset=%g reciprocal=%v)",
		c.Factor, c.Offset, c.Reciprocal)
}

// limitPrecision cuts the decimal places so that the largest value of the
// type has at most maxDigits digits. It uses a Java-style 32-bit int shift, so
// 32-bit unsigned cells (and axes without storage, width 0) are measured at
// raw 0.
func limitPrecision(v model.Value, bits int, signed bool) int {
	width := bits
	if signed {
		width--
	}
	raw := float64(int32(1)<<(uint(width)&31) - 1)
	if v.Conversion.Reciprocal {
		raw = 1
	}
	intDigits := javaInt(math.Floor(math.Log10(v.Conversion.Apply(raw))) + 1)
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

// subtracted returns the points of a "subtract" axis as WinOLS shows them:
// point i is 2^bits minus the sum of raw values i to n-1. Nil if the axis is
// float or doesn't fit in the image.
func subtracted(x *model.Axis, size int, image []byte) []float64 {
	w := x.Data.Bits / 8
	start := int(*x.Address)
	if w == 0 || x.Data.Float || start+size*w > len(image) {
		return nil
	}
	order := byteOrder(*x.Data)
	out := make([]float64, size)
	t := int64(1) << (8 * w)
	for i := size - 1; i >= 0; i-- {
		t -= rawInt(image[start+i*w:], order, w, x.Data.Signed)
		out[i] = float64(t)
	}
	return out
}

// rawInt reads one integer cell. A signed cell is sign-extended.
func rawInt(p []byte, order binary.ByteOrder, w int, signed bool) int64 {
	var u uint64
	switch w {
	case 1:
		u = uint64(p[0])
	case 2:
		u = uint64(order.Uint16(p))
	default:
		u = uint64(order.Uint32(p))
	}
	if !signed {
		return int64(u)
	}
	return int64(u<<(64-8*w)) >> (64 - 8*w)
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

// xdfVisLevel is the XDF vislevel attribute: the model's level when set,
// otherwise 2 (unmarked; level 1 is the WinOLS map-list marker).
func xdfVisLevel(o *model.Object) string {
	if o.VisLevel > 0 {
		return strconv.Itoa(o.VisLevel)
	}
	return "2"
}

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
