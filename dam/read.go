// Package dam reads an ASAP2DAM file into the model.
// Axes and conversions are inlined (docs/model.md). There is no public
// codec for these files; record types and the /REP coefficient order follow
// docs/model.md.
package dam

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.nyet.org/xdfkit/model"
)

// Read parses a DAMOS file. name is the provenance file name.
func Read(b []byte, name string) (*model.Model, []string, error) {
	text := latin1(b)
	if !strings.Contains(text[:min(len(text), 4096)], "ASAP2DAM") {
		return nil, nil, fmt.Errorf("dam: not an ASAP2DAM file")
	}
	lines := strings.Split(text, "\n")
	regs := map[int]*reg{}
	var cur *reg
	var proj, ver string
	var recs []record
	var open *record
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		f := fields(line)
		if len(f) == 0 {
			continue
		}
		kw := f[0]
		if len(f) > 1 && strings.HasPrefix(f[1], "/") {
			kw = f[1]
		}
		switch kw {
		case "/EPR":
			if len(f) > 1 {
				proj = brace(f[1])
			}
			if len(f) > 2 {
				ver = brace(f[2])
			}
		case "/REG":
			id, name, unit := 0, "", ""
			if len(f) > 0 && !strings.HasPrefix(f[0], "/") {
				id, _ = strconv.Atoi(f[0])
			}
			// id, /REG, name, {desc}, ..., {units}
			idx := indexOf(f, "/REG")
			if idx >= 0 && idx+1 < len(f) {
				name = f[idx+1]
			}
			for _, p := range f {
				if strings.HasPrefix(p, "{") && p != "{}" && unit == "" && p != f[idx+2] {
					unit = brace(p)
				}
			}
			cur = &reg{name: name, unit: unit}
			regs[id] = cur
		case "/REP":
			if cur == nil {
				continue
			}
			cur.c, cur.warn = rep(f)
		case "/SPZ":
			if r, ok := parseSPZ(f); ok {
				recs = append(recs, r)
				open = &recs[len(recs)-1]
			}
		case "/SPW", "/SPX", "/SPY", "/FKX", "/FKY":
			if open == nil {
				continue
			}
			switch kw {
			case "/SPW":
				open.w = f
			case "/SPX":
				open.x = f
			case "/SPY":
				open.y = f
			case "/FKX":
				open.fkx = f
			case "/FKY":
				open.fky = f
			}
		}
	}
	var warnings []string
	warnings = append(warnings, "byte order unconfirmed; stored as little-endian")
	var skipped map[string]int
	note := func(s string) { skipped = add(skipped, s) }
	tables := map[int]record{}
	for _, rec := range recs {
		if rec.typ == 6 && rec.index != 0 {
			tables[rec.index] = rec
		}
	}
	var objs []*model.Object
	for _, rec := range recs {
		o, skip, ws := emit(rec, regs, tables)
		warnings = append(warnings, ws...)
		if o == nil {
			if skip != "" {
				note(skip)
			}
			continue
		}
		objs = append(objs, o)
	}
	sum := sha256.Sum256(b)
	m := &model.Model{
		Schema:     model.SchemaID,
		Provenance: &model.Provenance{Format: "damos", Origin: "damos", File: name, SHA256: hex.EncodeToString(sum[:])},
		Project:    model.Project{Name: proj, Version: ver},
		Objects:    objs,
	}
	m.AssignKeys()
	var kinds []string
	for k := range skipped {
		kinds = append(kinds, k)
	}
	slices.Sort(kinds)
	for _, k := range kinds {
		warnings = append(warnings, fmt.Sprintf("%d %s skipped", skipped[k], k))
	}
	return m, capWarnings(warnings), m.Check()
}

type reg struct {
	name, unit, warn string
	c                model.Conversion
}

type record struct {
	index             int
	name, desc        string
	typ               int
	addr              uint32
	w, x, y, fkx, fky []string
}

type vec struct {
	id, width, n int
	min, max     string
	ok           bool
}

func parseSPZ(f []string) (record, bool) {
	i := indexOf(f, "/SPZ")
	if i < 0 || i+4 >= len(f) {
		return record{}, false
	}
	typ, err := strconv.Atoi(f[i+3])
	if err != nil {
		return record{}, false
	}
	addr, ok := parseHex(f[i+4])
	if !ok {
		return record{}, false
	}
	index := 0
	if i > 0 {
		index, _ = strconv.Atoi(f[0])
	}
	return record{index: index, name: f[i+1], desc: brace(f[i+2]), typ: typ, addr: addr}, true
}

func parseVec(f []string) vec {
	// /SPW, id, width, min, max
	// /SPX, id, width, min, max, n
	// /FKX, n, ...
	v := vec{ok: true}
	if len(f) > 1 {
		v.id, _ = strconv.Atoi(f[1])
	}
	if len(f) > 2 {
		v.width, _ = strconv.Atoi(f[2])
	}
	if len(f) > 3 {
		v.min = f[3]
	}
	if len(f) > 4 {
		v.max = f[4]
	}
	if len(f) > 5 {
		v.n, _ = strconv.Atoi(strings.TrimRight(f[5], ";"))
	} else if f[0] == "/FKX" || (len(f) > 1 && f[0] != "/SPW" && f[0] != "/SPX" && f[0] != "/SPY") {
		v.n = v.id
	}
	if strings.HasPrefix(f[0], "/FK") {
		v.n = v.id
	}
	return v
}

func emit(rec record, regs map[int]*reg, tables map[int]record) (*model.Object, string, []string) {
	off, ok := lower(rec.addr)
	if !ok {
		return nil, "", []string{rec.name + ": address not in the 0x800000 window"}
	}
	switch rec.typ {
	case 1, 2, 3, 4, 5, 6, 7, 8, 13:
	case 12:
		return nil, "type 12", nil
	default:
		return nil, fmt.Sprintf("type %d", rec.typ), nil
	}
	w := parseVec(prepend("/SPW", rec.w))
	x := parseVec(prepend("/SPX", rec.x))
	y := parseVec(prepend("/SPY", rec.y))
	fk := parseVec(prepend("/FKX", rec.fkx))
	bits, signed, wok := widthBits(w.width)
	xbits, xsigned, xok := widthBits(x.width)
	ybits, ysigned, yok := widthBits(y.width)
	if rec.typ == 6 {
		bits, signed, wok = xbits, xsigned, xok
	}
	if !wok || bits == 0 {
		return nil, "unconfirmed width", []string{fmt.Sprintf("%s: value width %d is unconfirmed", rec.name, w.width)}
	}
	if !xok && x.width != 0 {
		return nil, "unconfirmed width", nil
	}
	if !yok && y.width != 0 {
		return nil, "unconfirmed width", nil
	}
	c := regs[w.id]
	if rec.typ == 6 {
		c = regs[x.id]
	}
	var warnings []string
	val := valueFrom(c, w.min, w.max)
	if rec.typ == 6 {
		val = valueFrom(c, x.min, x.max)
	}
	if c != nil && c.warn != "" {
		warnings = append(warnings, rec.name+": "+c.warn)
	}
	o := &model.Object{
		ID: rec.name, Description: cleanText(rec.desc), Address: off,
		Data: dataOf(bits, signed), Value: val,
	}
	switch rec.typ {
	case 1:
		o.Shape, o.Rows, o.Cols = "value", 1, 1
	case 13:
		n := fk.n
		if n < 1 || n > 256 {
			n = 1
		}
		if n == 1 {
			o.Shape, o.Rows, o.Cols = "value", 1, 1
		} else {
			o.Shape, o.Rows, o.Cols = "1d", 1, n
			o.X = ordinalAxis()
		}
	case 2, 4, 6:
		n := fkCount(rec.fkx)
		if n < 1 {
			n = 1
		}
		o.Shape, o.Rows, o.Cols = "1d", 1, n
		if rec.typ == 6 {
			o.X = ordinalAxis()
		} else {
			o.X = axisFor(rec.typ, off, xbits, xsigned, regs[x.id], x)
		}
	case 7:
		o.Shape, o.Rows = "1d", 1
		if ax, ok := sharedAxis(tables, x.n, regs); ok {
			o.Cols = ax.n
			o.X = ax.axis
		} else {
			o.Cols = 1
			o.X = &model.Axis{Source: "unknown", Value: model.Value{Conversion: model.Conversion{Factor: 1}}}
			warnings = append(warnings, rec.name+": shared axis not found")
		}
	case 5:
		rows, cols := fkCount(rec.fkx), fkCount(rec.fky)
		if rows < 1 {
			rows = 1
		}
		if cols < 1 {
			cols = 1
		}
		o.Shape, o.Rows, o.Cols = "2d", rows, cols
		o.Y = axisFor(rec.typ, off, xbits, xsigned, regs[x.id], x)
		o.X = axisFor(rec.typ, off, ybits, ysigned, regs[y.id], y)
	case 3:
		// /FKX is the row count and /SPX its width; /FKY is the column count
		// and /SPY its width. An inline map stores the row axis first.
		rows, cols := fkCount(rec.fkx), fkCount(rec.fky)
		if rows < 1 {
			rows = 1
		}
		if cols < 1 {
			cols = 1
		}
		o.Shape, o.Rows, o.Cols = "2d", rows, cols
		o.Y = axisFor(rec.typ, off, xbits, xsigned, regs[x.id], x)
		o.X = axisFor(rec.typ, off, ybits, ysigned, regs[y.id], y)
		placeMap(o, off, xbits, ybits, bits)
	case 8:
		o.Shape = "2d"
		axY, yOK := sharedAxis(tables, x.n, regs)
		axX, xOK := sharedAxis(tables, y.n, regs)
		if !yOK || !xOK {
			warnings = append(warnings, rec.name+": shared axis not found")
		}
		if yOK {
			o.Rows = axY.n
			o.Y = axY.axis
		} else {
			o.Rows = 1
			o.Y = &model.Axis{Source: "unknown", Value: model.Value{Conversion: model.Conversion{Factor: 1}}}
		}
		if xOK {
			o.Cols = axX.n
			o.X = axX.axis
		} else {
			o.Cols = 1
			o.X = &model.Axis{Source: "unknown", Value: model.Value{Conversion: model.Conversion{Factor: 1}}}
		}
	}
	if rec.typ == 2 || rec.typ == 6 {
		placeCurve(o, off, xbits, bits, rec.typ == 6)
	}
	return o, "", warnings
}

// axisRef is a type 6 breakpoint series copied onto a shared-axis map.
type axisRef struct {
	n    int
	axis *model.Axis
}

// sharedAxis is the breakpoint series of a type 6 table. On a type 7 curve
// the last /SPX number is that table's /SPZ index; on a type 8 map /SPX is
// the row axis and /SPY the column axis.
func sharedAxis(tables map[int]record, index int, regs map[int]*reg) (axisRef, bool) {
	rec, ok := tables[index]
	if !ok {
		return axisRef{}, false
	}
	x := parseVec(prepend("/SPX", rec.x))
	bits, signed, wok := widthBits(x.width)
	n := fkCount(rec.fkx)
	off, lok := lower(rec.addr)
	if !wok || bits == 0 || n < 1 || !lok {
		return axisRef{}, false
	}
	p := int(off) + 1
	if bits >= 16 && p%2 != 0 {
		p++
	}
	ad := model.Addr(p)
	d := dataOf(bits, signed)
	return axisRef{n, &model.Axis{
		Source: "image", Stored: "absolute", Address: &ad, Data: &d,
		Value: valueFrom(regs[x.id], x.min, x.max),
	}}, true
}

func axisFor(typ int, base model.Addr, bits int, signed bool, c *reg, v vec) *model.Axis {
	val := valueFrom(c, v.min, v.max)
	switch typ {
	case 4, 5:
		return &model.Axis{Source: "ordinal", Value: val}
	default:
		ad := base
		d := dataOf(bits, signed)
		if bits == 0 {
			d = dataOf(8, false)
		}
		return &model.Axis{Source: "image", Stored: "absolute", Address: &ad, Data: &d, Value: val}
	}
}

func placeCurve(o *model.Object, base model.Addr, xbits, vbits int, axisIsValues bool) {
	p := int(base) + 1
	if xbits >= 16 && p%2 != 0 {
		p++
	}
	if o.X != nil && o.X.Address != nil && o.X.Source == "image" {
		a := model.Addr(p)
		o.X.Address = &a
	}
	if axisIsValues {
		o.Address = model.Addr(p)
		return
	}
	p += o.Cols * max(xbits, 8) / 8
	if vbits >= 16 && p%2 != 0 {
		p++
	}
	o.Address = model.Addr(p)
}

// placeMap lays out an inline map: a count per axis, the row axis, the column
// axis, then the values. Both counts are 16-bit when both axes are; otherwise
// each count is a byte. A field of 16 bits or more starts at an even address.
func placeMap(o *model.Object, base model.Addr, rowBits, colBits, vbits int) {
	cb := 8
	if rowBits >= 16 && colBits >= 16 {
		cb = 16
	}
	p := int(base)
	for range 2 {
		if cb >= 16 && p%2 != 0 {
			p++
		}
		p += cb / 8
	}
	if rowBits >= 16 && p%2 != 0 {
		p++
	}
	if o.Y != nil && o.Y.Source == "image" && o.Y.Address != nil {
		a := model.Addr(p)
		o.Y.Address = &a
	}
	p += o.Rows * max(rowBits, 8) / 8
	if colBits >= 16 && p%2 != 0 {
		p++
	}
	if o.X != nil && o.X.Source == "image" && o.X.Address != nil {
		a := model.Addr(p)
		o.X.Address = &a
	}
	p += o.Cols * max(colBits, 8) / 8
	if vbits >= 16 && p%2 != 0 {
		p++
	}
	o.Address = model.Addr(p)
}

func fkCount(f []string) int {
	if len(f) < 2 {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimRight(strings.TrimSpace(f[1]), ";"))
	if err != nil || n < 1 || n > 512 {
		return 0
	}
	return n
}

func widthBits(w int) (bits int, signed, ok bool) {
	if w == 0 {
		return 0, false, true
	}
	if w < 0 {
		signed = true
		w = -w
	}
	bits = w * 8
	ok = bits == 8 || bits == 16 || bits == 32
	return bits, signed, ok
}

func lower(addr uint32) (model.Addr, bool) {
	if addr >= 0x800000 && addr < 0x1000000 {
		return model.Addr(addr - 0x800000), true
	}
	if addr < 0x800000 {
		return model.Addr(addr), true
	}
	return 0, false
}

// rep reads A, B, C, D of internal = (A*phys + B) / (C*phys + D).
func rep(f []string) (model.Conversion, string) {
	nums := f[1:]
	if len(nums) < 4 {
		return model.Conversion{Factor: 1}, "conversion is not linear or reciprocal; read as X"
	}
	a := num(nums[0])
	b := num(nums[1])
	c := num(nums[2])
	d := num(nums[3])
	z := func(x float64) bool { return math.Abs(x) < 1e-12 }
	switch {
	case z(a) && z(d) && !z(c):
		return model.Conversion{Factor: b / c, Reciprocal: true}, ""
	case z(c) && !z(d) && !z(a):
		return model.Conversion{Factor: d / a, Offset: -b / a}, ""
	default:
		return model.Conversion{Factor: 1}, "conversion is not linear or reciprocal; read as X"
	}
}

func valueFrom(c *reg, minS, maxS string) model.Value {
	conv := model.Conversion{Factor: 1}
	unit := ""
	if c != nil {
		conv = c.c
		unit = c.unit
		if conv.Factor == 0 && !conv.Reciprocal {
			conv.Factor = 1
		}
	}
	v := model.Value{Units: unit, Precision: decimals(minS, maxS), Conversion: conv}
	if minS != "" {
		if x, err := strconv.ParseFloat(minS, 64); err == nil {
			v.Min = &x
		}
	}
	if maxS != "" {
		if x, err := strconv.ParseFloat(maxS, 64); err == nil {
			v.Max = &x
		}
	}
	return v
}

func dataOf(bits int, signed bool) model.Data {
	if bits == 0 {
		bits = 8
	}
	d := model.Data{Bits: bits, Signed: signed}
	if bits > 8 {
		d.Endian = "little"
	}
	return d
}

func ordinalAxis() *model.Axis {
	return &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}}
}

func decimals(a, b string) int {
	n := dec(a)
	if m := dec(b); m > n {
		return m
	}
	return n
}

func dec(s string) int {
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	t := strings.TrimRight(s[i+1:], "0")
	return len(t)
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimRight(s, ";"), 64)
	return f
}

func prepend(kw string, f []string) []string {
	if len(f) == 0 {
		return []string{kw}
	}
	if f[0] == kw {
		return f
	}
	return append([]string{kw}, f...)
}

func indexOf(f []string, s string) int {
	for i, p := range f {
		if p == s {
			return i
		}
	}
	return -1
}

func brace(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	return s
}

func parseHex(s string) (uint32, bool) {
	s = strings.TrimRight(strings.TrimSpace(s), ";")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "$"), "0x")
	n, err := strconv.ParseUint(s, 16, 32)
	return uint32(n), err == nil && s != ""
}

func fields(line string) []string {
	var out []string
	var buf []rune
	depth := 0
	for _, c := range line {
		switch {
		case c == '{':
			depth++
			buf = append(buf, c)
		case c == '}':
			if depth > 0 {
				depth--
			}
			buf = append(buf, c)
		case c == ',' && depth == 0:
			out = append(out, strings.TrimSpace(string(buf)))
			buf = buf[:0]
		default:
			buf = append(buf, c)
		}
	}
	if len(buf) > 0 {
		s := strings.TrimSpace(string(buf))
		s = strings.TrimRight(s, ";")
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

func add(m map[string]int, k string) map[string]int {
	if m == nil {
		m = map[string]int{}
	}
	m[k]++
	return m
}

// cleanText drops tabs from DAMOS display text (often TAB after a colon).
func cleanText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\t", " "))
}

func capWarnings(ws []string) []string {
	n := map[string]int{}
	var out []string
	for _, w := range ws {
		key := w
		if i := strings.Index(w, ": "); i >= 0 {
			key = w[i+2:]
		}
		n[key]++
		if n[key] <= 3 {
			out = append(out, w)
		}
	}
	var extra []string
	for k, c := range n {
		if c > 3 {
			extra = append(extra, fmt.Sprintf("%d more: %s", c-3, k))
		}
	}
	slices.Sort(extra)
	return append(out, extra...)
}
