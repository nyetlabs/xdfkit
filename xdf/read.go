package xdf

import (
	"bytes"
	"cmp"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.nyet.org/xdfkit/model"
)

// rdoc is the part of an XDF the reader uses. Other elements, including
// TunerPro's own additions, are ignored.
type rdoc struct {
	XMLName xml.Name `xml:"XDFFORMAT"`
	Header  struct {
		FileVersion string `xml:"fileversion"`
		Description string `xml:"description"`
		Defaults    struct {
			SigDigits *int `xml:"sigdigits,attr"`
		} `xml:"DEFAULTS"`
		Categories []category `xml:"CATEGORY"`
	} `xml:"XDFHEADER"`
	Objects []robject `xml:",any"`
}

// robject is an XDFTABLE or XDFCONSTANT; a constant's cells are in its
// embedded raxis.
type robject struct {
	XMLName     xml.Name
	UniqueID    string        `xml:"uniqueid,attr"`
	Title       string        `xml:"title"`
	Description string        `xml:"description"`
	CategoryMem []categoryMem `xml:"CATEGORYMEM"`
	raxis
	Axes []raxis `xml:"XDFAXIS"`
}

type raxis struct {
	ID         string   `xml:"id,attr"`
	Data       embedded `xml:"EMBEDDEDDATA"`
	Units      string   `xml:"units"`
	IndexCount int      `xml:"indexcount"`
	DecimalPl  *int     `xml:"decimalpl"`
	OutputType string   `xml:"outputtype"`
	Math       equation `xml:"MATH"`
}

// stampPrefix starts the stamp line in the header description, which the
// view leaves out (docs/stamp-and-metadata.md).
const stampPrefix = "xdfkit-stamp:"

// subsetPrefix starts the line in the header description that marks a subset
// ("xdfkit-subset: tuner sha256:<table>"); the view reads it into
// provenance.subset, so the mark survives without the metadata file.
const subsetPrefix = "xdfkit-subset:"

// view is the XDF view: the model read from the XDF elements alone, objects
// in uniqueid order (numeric; others after them, in file order), with the
// uniqueid of each object. Label values are not read: the model has no
// field for them.
func view(b []byte) (m *model.Model, uids, warnings []string, err error) {
	if !utf8.Valid(b) {
		b = fromCP1252(b)
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var d rdoc
	if err := dec.Decode(&d); err != nil {
		return nil, nil, nil, err
	}
	h := &d.Header
	m = &model.Model{Schema: model.SchemaID, Objects: []*model.Object{}}
	var desc []string
	for _, l := range strings.Split(trim(h.Description), "\n") {
		if s, ok := strings.CutPrefix(strings.TrimSuffix(l, "\r"), subsetPrefix); ok {
			kind, table, _ := strings.Cut(strings.TrimSpace(s), " sha256:")
			m.Provenance = &model.Provenance{Subset: &model.Subset{Kind: kind, Table: table}}
			continue
		}
		if !strings.HasPrefix(l, stampPrefix) {
			desc = append(desc, l)
		}
	}
	m.Project = model.Project{Name: trim(strings.Join(desc, "\n")), Version: trim(h.FileVersion)}
	cats := map[int]bool{}
	for _, c := range h.Categories {
		id, err := parseHex(c.Index)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("category %q: index %q ignored", c.Name, c.Index))
			continue
		}
		m.Categories = append(m.Categories, &model.Category{ID: int(id), Name: trim(c.Name)})
		cats[int(id)] = true
	}
	prec := 2
	if h.Defaults.SigDigits != nil {
		prec = *h.Defaults.SigDigits
	}
	type entry struct {
		uid string
		n   uint64
		ok  bool
		o   *model.Object
	}
	var es []entry
	ignored := map[string]int{}
	for i := range d.Objects {
		r := &d.Objects[i]
		if r.XMLName.Local != "XDFTABLE" && r.XMLName.Local != "XDFCONSTANT" {
			ignored[r.XMLName.Local]++
			continue
		}
		o, ws, err := r.object(prec, cats)
		name := fmt.Sprintf("%s %s %q", r.XMLName.Local, r.UniqueID, trim(r.Title))
		for _, w := range ws {
			warnings = append(warnings, name+": "+w)
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		n, err := parseHex(r.UniqueID)
		es = append(es, entry{r.UniqueID, n, err == nil, o})
	}
	for _, k := range slices.Sorted(maps.Keys(ignored)) {
		warnings = append(warnings, fmt.Sprintf("%d %s elements ignored", ignored[k], k))
	}
	slices.SortStableFunc(es, func(a, b entry) int {
		switch {
		case a.ok == b.ok:
			return cmp.Compare(a.n, b.n)
		case a.ok:
			return -1
		}
		return 1
	})
	for _, e := range es {
		m.Objects = append(m.Objects, e.o)
		uids = append(uids, e.uid)
	}
	m.AssignKeys()
	return m, uids, warnings, nil
}

// object converts an XDFTABLE or XDFCONSTANT; cats holds the category ids.
func (r *robject) object(prec int, cats map[int]bool) (*model.Object, []string, error) {
	o := &model.Object{ID: trim(r.Title), Description: trim(r.Description)}
	for _, c := range r.CategoryMem {
		if cats[c.Category-1] && !slices.Contains(o.Categories, c.Category-1) {
			o.Categories = append(o.Categories, c.Category-1)
		}
	}
	z := &r.raxis
	if r.XMLName.Local == "XDFCONSTANT" {
		o.Shape, o.Rows, o.Cols = "value", 1, 1
	} else {
		i := slices.IndexFunc(r.Axes, func(a raxis) bool { return a.ID == "z" })
		if i < 0 {
			return nil, nil, errors.New("no z axis")
		}
		z = &r.Axes[i]
		o.Rows, o.Cols = max(z.Data.RowCount, 1), max(z.Data.ColCount, 1)
		o.Shape = "1d"
		if o.Rows > 1 {
			o.Shape = "2d"
		}
	}
	addr, err := parseHex(z.Data.Address)
	if err != nil || addr > 0xFFFFFFFF {
		return nil, nil, fmt.Errorf("cell address %q", z.Data.Address)
	}
	var warnings []string
	o.Address, o.Data = model.Addr(addr), z.Data.data()
	o.Value, o.View, err = z.value(prec)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	if o.Shape != "value" {
		for _, a := range []struct {
			id string
			x  **model.Axis
		}{{"x", &o.X}, {"y", &o.Y}} {
			if *a.x, err = r.axis(a.id, prec); err != nil {
				warnings = append(warnings, fmt.Sprintf("axis %s: %v", a.id, err))
			}
		}
	}
	return o, warnings, nil
}

// axis converts a table's x or y axis: ordinal for labels (nil for one label
// or none), else an image axis.
func (r *robject) axis(id string, prec int) (*model.Axis, error) {
	i := slices.IndexFunc(r.Axes, func(a raxis) bool { return a.ID == id })
	if i < 0 {
		return nil, nil
	}
	a := &r.Axes[i]
	v, view, err := a.value(prec)
	x := &model.Axis{Source: "ordinal", Value: v, View: view}
	if a.Data.Address == "" {
		if a.IndexCount <= 1 {
			return nil, err
		}
		return x, err
	}
	addr, aerr := parseHex(a.Data.Address)
	if aerr != nil || addr > 0xFFFFFFFF {
		return nil, fmt.Errorf("address %q", a.Data.Address)
	}
	ad, d := model.Addr(addr), a.Data.data()
	x.Source, x.Stored, x.Address, x.Data = "image", "absolute", &ad, &d
	return x, err
}

// value reads the units, decimal places (default prec), output type (3 is
// hex, the others decimal) and equation; an equation that isn't linear in X
// reads as X, with the error.
func (a *raxis) value(prec int) (model.Value, model.View, error) {
	if a.DecimalPl != nil {
		prec = *a.DecimalPl
	}
	c, err := parseEquation(a.Math.Equation)
	if err != nil {
		c, err = model.Conversion{Factor: 1}, fmt.Errorf("equation %q: %v; read as X", a.Math.Equation, err)
	}
	v := model.View{Base: 10}
	if strings.TrimSpace(a.OutputType) == "3" {
		v.Base = 16
	}
	return model.Value{Units: trim(a.Units), Precision: prec, Conversion: c}, v, err
}

func (e embedded) data() model.Data {
	f, _ := parseHex(e.TypeFlags)
	d := model.Data{Bits: e.ElementSizeBits, Signed: f&flagSigned != 0, Float: f&flagFloat != 0}
	if d.Bits == 0 {
		d.Bits = 8
	}
	if d.Bits > 8 {
		d.Endian = map[bool]string{true: "little", false: "big"}[f&flagLE != 0]
	}
	return d
}

// parseHex reads TunerPro's "0x..." numbers.
func parseHex(s string) (uint64, error) {
	h, ok := strings.CutPrefix(strings.TrimSpace(s), "0x")
	if !ok {
		h, ok = strings.CutPrefix(strings.TrimSpace(s), "0X")
	}
	if !ok {
		return 0, fmt.Errorf("%q is not 0x hex", s)
	}
	return strconv.ParseUint(h, 16, 64)
}

// cp1252 holds the Windows-1252 characters at 0x80 to 0x9F; the undefined
// ones map to the same code point, as Windows does.
var cp1252 = []rune("€\u0081‚ƒ„…†‡ˆ‰Š‹Œ\u008DŽ\u008F\u0090‘’“”•–—˜™š›œ\u009DžŸ")

// fromCP1252 decodes Windows-1252, which TunerPro assumes for XDF without a
// declaration.
func fromCP1252(b []byte) []byte {
	var out bytes.Buffer
	for _, c := range b {
		switch {
		case c < 0x80:
			out.WriteByte(c)
		case c < 0xA0:
			out.WriteRune(cp1252[c-0x80])
		default:
			out.WriteRune(rune(c))
		}
	}
	return out.Bytes()
}
