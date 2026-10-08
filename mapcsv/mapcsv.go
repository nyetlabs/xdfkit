// Package mapcsv writes a model as the map list CSV that ecuxplot's mapdump
// printed (docs/design.md): one row per object in address order, with value
// ranges read from the image and the names of matching maps in reference
// definitions.
//
// It follows mapdump's columns and spelling (numbers as Java's Double.toString,
// empty fields as "-"), with deliberate differences: the raw minimum and
// maximum of 32-bit cells are masked to 32 bits (mapdump always printed 0x0),
// float cells are read as floats, and objects without cells get "-" instead of
// mapdump's ±9.223372036854776E18.
package mapcsv

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.nyet.org/xdfkit/model"
)

// Ref is a reference definition: its maps' names fill one column per Ref, for
// the objects whose id matches. Name is the column header.
type Ref struct {
	Name  string
	Model *model.Model
}

var header = []string{
	"ID", "Address", "Name", "Size", "Organization", "Description",
	"Units", "X Address", "Y Address", "X Units", "Y Units",
	"Scale", "X Scale", "Y Scale",
	"Value min", "Value max", "Value min*1", "Value max*1",
}

// Write returns m as CSV. Without an image the value columns are "-".
func Write(m *model.Model, image []byte, refs []Ref) ([]byte, error) {
	var b strings.Builder
	row := func(fields []string) {
		for i, f := range fields {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`"` + strings.ReplaceAll(f, `"`, `""`) + `"`)
		}
		b.WriteByte('\n')
	}
	h := header
	for _, r := range refs {
		h = append(h[:len(h):len(h)], r.Name)
	}
	row(h)
	for _, o := range byAddress(m) {
		f := []string{
			o.ID, hex(uint64(o.Address)), o.Description, fmt.Sprintf("%dx%d", o.Cols, o.Rows), typeName(o.Data),
			o.Value.Description, o.Value.Units, axisAddr(o.X), axisAddr(o.Y), axisUnits(o.X), axisUnits(o.Y),
			javaDouble(o.Value.Conversion.Factor), axisScale(o.X), axisScale(o.Y),
		}
		if len(image) > 0 && o.Rows*o.Cols > 0 {
			r, err := ranges(o, image)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", o.Key, err)
			}
			f = append(f, r...)
		} else {
			f = append(f, "", "", "", "")
		}
		f = append(f, o.Comment)
		for i, s := range f {
			if s == "" {
				f[i] = "-"
			}
		}
		for _, r := range refs {
			f = append(f, refName(o, r.Model))
		}
		row(f)
	}
	return []byte(b.String()), nil
}

// byAddress returns the objects sorted by address, in source order at the
// same address.
func byAddress(m *model.Model) []*model.Object {
	return slices.SortedStableFunc(slices.Values(m.Objects), func(a, b *model.Object) int {
		return cmp.Compare(a.Address, b.Address)
	})
}

func hex(v uint64) string { return fmt.Sprintf("0x%x", v) }

func typeName(d model.Data) string {
	order := map[string]string{"big": "HiLo", "little": "LoHi"}[d.Endian]
	switch {
	case d.Bits == 8:
		return "8 Bit"
	case d.Bits == 16:
		return "16 Bit (" + order + ")"
	case d.Float:
		return "32 BitFloat (" + order + order + ")"
	}
	return "32 Bit (" + order + order + ")"
}

func axisAddr(a *model.Axis) string {
	if a == nil || a.Address == nil {
		return ""
	}
	return hex(uint64(*a.Address))
}

func axisUnits(a *model.Axis) string {
	if a == nil {
		return ""
	}
	return a.Value.Units
}

func axisScale(a *model.Axis) string {
	if a == nil {
		return ""
	}
	return javaDouble(a.Value.Conversion.Factor)
}

// ranges reads the object's cells and returns the converted minimum and
// maximum and the raw minimum and maximum in hex.
func ranges(o *model.Object, image []byte) ([]string, error) {
	w := o.Data.Bits / 8
	n := o.Rows * o.Cols
	start := int(o.Address)
	if start+n*w > len(image) {
		return nil, fmt.Errorf("cells at %s run past the image end", hex(uint64(o.Address)))
	}
	var order binary.ByteOrder = binary.LittleEndian
	if o.Data.Endian == "big" {
		order = binary.BigEndian
	}
	var lo, hi float64
	var rlo, rhi uint64
	for i := range n {
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
		v := float64(u)
		switch {
		case o.Data.Float:
			v = float64(math.Float32frombits(uint32(u)))
		case o.Data.Signed:
			v = float64(int64(u<<(64-8*w)) >> (64 - 8*w))
		}
		if i == 0 || v < lo {
			lo, rlo = v, u
		}
		if i == 0 || v > hi {
			hi, rhi = v, u
		}
	}
	a, b := convert(o.Value.Conversion, lo), convert(o.Value.Conversion, hi)
	return []string{javaDouble(math.Min(a, b)), javaDouble(math.Max(a, b)), hex(rlo), hex(rhi)}, nil
}

func convert(c model.Conversion, raw float64) float64 {
	if c.Reciprocal {
		return c.Factor/raw + c.Offset
	}
	return float64(raw*c.Factor) + c.Offset // no fused multiply-add, as in Java
}

// refName is the name (KP name, model description) of the first object in ref
// whose id stem equals o's; the stem is the id up to the first '?' or space.
func refName(o *model.Object, ref *model.Model) string {
	stem := func(id string) string {
		if i := strings.IndexAny(id, "? "); i >= 0 {
			return id[:i]
		}
		return id
	}
	s := stem(o.ID)
	if s == "" {
		return ""
	}
	for _, r := range byAddress(ref) {
		if stem(r.ID) == s {
			return r.Description
		}
	}
	return ""
}

// javaDouble spells f as Java's Double.toString does: the shortest digits that
// identify f, plain from 1e-3 up to 1e7, else as d.dddE±n, always with a
// fractional digit.
func javaDouble(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	}
	if a := math.Abs(f); a == 0 || (a >= 1e-3 && a < 1e7) {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return s
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e")
	if !strings.Contains(mant, ".") {
		mant += ".0"
	}
	e, _ := strconv.Atoi(exp)
	return mant + "E" + strconv.Itoa(e)
}
