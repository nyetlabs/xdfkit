// Package a2l reads an ASAM MCD-2 MC (ASAP2) file into the model.
// Shared axes, conversions and record layouts are inlined onto each object
// (docs/model.md). Measurements, formulas and enum tables are skipped.
package a2l

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

// Read parses an A2L file. name is the provenance file name.
func Read(b []byte, name string) (*model.Model, []string, error) {
	toks := tokenize(b)
	if !has(toks, "ASAP2_VERSION") {
		return nil, nil, fmt.Errorf("a2l: not an ASAP2 file")
	}
	r := &reader{alignW: 1, alignL: 1, little: true}
	r.comps = map[string]compu{}
	r.layouts = map[string][]member{}
	r.axes = map[string]*axisPts{}
	var funcs []function
	var warnings []string
	var skipped map[string]int
	note := func(s string) { skipped = add(skipped, s) }
	blocks := allBlocks(toks)

	for _, bl := range blocks {
		switch bl.name {
		case "COMPU_METHOD":
			if c, ok := parseCompu(bl.body); ok {
				r.comps[c.name] = c
			}
		case "RECORD_LAYOUT":
			if n, ms, ok := parseLayout(bl.body); ok {
				r.layouts[n] = ms
			}
		case "AXIS_PTS":
			if a, ok := parseAxisPts(bl.body); ok {
				r.axes[a.name] = a
			}
		case "MOD_COMMON":
			r.modCommon(bl.body)
		case "MEMORY_SEGMENT":
			if s, ok := parseSeg(bl.body); ok {
				r.segs = append(r.segs, s)
			}
		case "FUNCTION", "GROUP":
			if f, ok := parseFunc(bl.body); ok {
				funcs = append(funcs, f)
			}
		case "MEASUREMENT":
			note("MEASUREMENT")
		case "PROJECT":
			r.project(bl.body)
		}
	}
	catOf := map[string][]int{}
	var cats []*model.Category
	for i, f := range funcs {
		cats = append(cats, &model.Category{ID: i, Name: f.name})
		for _, n := range f.chars {
			catOf[n] = append(catOf[n], i)
		}
	}
	var objs []*model.Object
	seen := map[string]bool{}
	for _, bl := range blocks {
		if bl.name != "CHARACTERISTIC" {
			continue
		}
		o, skip, ws := r.characteristic(bl.body)
		warnings = append(warnings, ws...)
		if o == nil {
			if skip != "" {
				note(skip)
			}
			continue
		}
		o.Categories = catOf[o.ID]
		objs = append(objs, o)
		seen[o.ID] = true
	}
	var axisNames []string
	for n := range r.axes {
		axisNames = append(axisNames, n)
	}
	slices.Sort(axisNames)
	for _, n := range axisNames {
		if seen[n] {
			continue
		}
		o, ok := r.axisObject(r.axes[n])
		if !ok {
			continue
		}
		o.Categories = catOf[n]
		objs = append(objs, o)
	}
	sum := sha256.Sum256(b)
	m := &model.Model{
		Schema:     model.SchemaID,
		Provenance: &model.Provenance{Format: "a2l", Origin: "a2l", File: name, SHA256: hex.EncodeToString(sum[:])},
		Project:    model.Project{Name: r.proj, Version: r.ver},
		Categories: cats,
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
	if r.unlowered > 0 {
		warnings = append(warnings, fmt.Sprintf("%d addresses outside a known segment left unread", r.unlowered))
	}
	return m, capWarnings(warnings), m.Check()
}

type reader struct {
	comps     map[string]compu
	layouts   map[string][]member
	axes      map[string]*axisPts
	segs      []segment
	alignW    int
	alignL    int
	little    bool
	proj      string
	ver       string
	unlowered int
}

type compu struct {
	name, unit, kind string
	prec             int
	c                model.Conversion
	warn             string
}

type member struct {
	kind        string
	pos         int
	bits        int
	signed, flt bool
}

type axisPts struct {
	name, layout, conv string
	addr               uint32
	max                int
}

type segment struct{ addr, size uint32 }

type function struct {
	name  string
	chars []string
}

type block struct {
	name string
	body []token
}

func (r *reader) characteristic(body []token) (*model.Object, string, []string) {
	if len(body) < 9 {
		return nil, "CHARACTERISTIC", []string{"characteristic: short record"}
	}
	name := body[0].s
	desc := cleanText(unquote(body[1]))
	kind := body[2].s
	addr, ok := parseAddr(body[3].s)
	if !ok {
		return nil, "", []string{name + ": address " + body[3].s}
	}
	layout := body[4].s
	conv := body[6].s
	lo, _ := strconv.ParseFloat(body[7].s, 64)
	hi, _ := strconv.ParseFloat(body[8].s, 64)
	switch kind {
	case "VALUE", "CURVE", "MAP", "VAL_BLK":
	default:
		return nil, kind, nil
	}
	rest := body[9:]
	prec := r.comps[conv].prec
	number := 1
	var axes []axisDesc
	var warnings []string
	for i := 0; i < len(rest); i++ {
		switch rest[i].s {
		case "FORMAT":
			if i+1 < len(rest) {
				prec = precision(rest[i+1].s)
			}
		case "NUMBER":
			if i+1 < len(rest) {
				number, _ = strconv.Atoi(rest[i+1].s)
			}
		case "/begin":
			if i+1 < len(rest) && rest[i+1].s == "AXIS_DESCR" {
				inner, end := untilEnd(rest, i)
				axes = append(axes, parseAxisDescr(inner))
				i = end
			} else {
				_, end := untilEnd(rest, i)
				i = end
			}
		}
	}
	off, ok := r.lower(addr)
	if !ok {
		r.unlowered++
		return nil, "", nil
	}
	ms := r.layouts[layout]
	fn := findMember(ms, "FNC_VALUES")
	if fn.bits == 0 {
		fn = member{bits: 8}
		warnings = append(warnings, name+": record layout "+layout+" has no FNC_VALUES; 8-bit assumed")
	}
	if fn.bits != 8 && fn.bits != 16 && fn.bits != 32 {
		return nil, "", []string{fmt.Sprintf("%s: %d-bit cells are not in the model", name, fn.bits)}
	}
	c := r.comps[conv]
	if c.warn != "" {
		warnings = append(warnings, name+": "+c.warn)
	}
	o := &model.Object{
		ID: name, Description: desc, Address: off,
		Data:  dataOf(fn, r.little),
		Value: valueOf(c, prec, &lo, &hi),
	}
	pts := map[string]int{}
	for i, a := range axes {
		key := "AXIS_PTS_X"
		if i == 1 {
			key = "AXIS_PTS_Y"
		}
		pts[key] = a.max
	}
	valOff, axisOff := place(ms, pts, r.alignW, r.alignL)
	o.Address = off + model.Addr(valOff)
	switch kind {
	case "VALUE":
		o.Shape, o.Rows, o.Cols = "value", 1, 1
	case "VAL_BLK":
		if number < 1 {
			number = 1
		}
		o.Shape, o.Rows, o.Cols = "1d", 1, number
		o.X = &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}}
	case "CURVE":
		n := 1
		if len(axes) > 0 && axes[0].max > 0 {
			n = axes[0].max
		}
		o.Shape, o.Rows, o.Cols = "1d", 1, n
	default:
		nx, ny := 1, 1
		if len(axes) > 0 && axes[0].max > 0 {
			nx = axes[0].max
		}
		if len(axes) > 1 && axes[1].max > 0 {
			ny = axes[1].max
		}
		o.Shape, o.Rows, o.Cols = "2d", ny, nx
	}
	for i, a := range axes {
		ax, w := r.axis(a, off, axisOff, i, ms)
		warnings = append(warnings, w...)
		if i == 0 {
			o.X = ax
		} else if i == 1 {
			o.Y = ax
		}
	}
	return o, "", warnings
}

type axisDesc struct {
	attr, conv, ref string
	max             int
	fix             bool
}

func parseAxisDescr(body []token) axisDesc {
	a := axisDesc{}
	if len(body) > 0 {
		a.attr = body[0].s
	}
	if len(body) > 2 {
		a.conv = body[2].s
	}
	if len(body) > 3 {
		a.max, _ = strconv.Atoi(body[3].s)
	}
	for i := 0; i < len(body); i++ {
		switch body[i].s {
		case "AXIS_PTS_REF":
			if i+1 < len(body) {
				a.ref = body[i+1].s
			}
		case "FIX_AXIS_PAR", "FIX_AXIS_PAR_DIST":
			a.fix = true
		}
	}
	return a
}

func (r *reader) axis(a axisDesc, base model.Addr, axisOff map[string]int, index int, ms []member) (*model.Axis, []string) {
	key := "AXIS_PTS_X"
	if index == 1 {
		key = "AXIS_PTS_Y"
	}
	c := r.comps[a.conv]
	v := valueOf(c, c.prec, nil, nil)
	switch a.attr {
	case "COM_AXIS":
		ap := r.axes[a.ref]
		if ap == nil {
			return &model.Axis{Source: "unknown", Value: v}, []string{"COM_AXIS " + a.ref + " not found"}
		}
		off, ok := r.lower(ap.addr)
		if !ok {
			r.unlowered++
			return &model.Axis{Source: "unknown", Value: v}, nil
		}
		_, at := place(r.layouts[ap.layout], map[string]int{"AXIS_PTS_X": ap.max}, r.alignW, r.alignL)
		delta := at["AXIS_PTS_X"]
		if delta == 0 && findMember(r.layouts[ap.layout], "FNC_VALUES").bits != 0 {
			// layout stores the points as FNC_VALUES
		}
		mem := findMember(r.layouts[ap.layout], "AXIS_PTS_X")
		if mem.bits == 0 {
			mem = findMember(r.layouts[ap.layout], "FNC_VALUES")
		}
		ad := off + model.Addr(delta)
		d := dataOf(mem, r.little)
		return &model.Axis{Source: "image", Stored: "absolute", Address: &ad, Data: &d, Value: v}, nil
	case "FIX_AXIS":
		return &model.Axis{Source: "ordinal", Value: v}, nil
	case "STD_AXIS":
		mem := findMember(ms, key)
		delta, ok := axisOff[key]
		if !ok || mem.bits == 0 {
			ad := base
			return &model.Axis{Source: "image", Stored: "absolute", Address: &ad, Value: v}, []string{key + " not in the record layout; axis address is the record start"}
		}
		ad := base + model.Addr(delta)
		d := dataOf(mem, r.little)
		return &model.Axis{Source: "image", Stored: "absolute", Address: &ad, Data: &d, Value: v}, nil
	default:
		return &model.Axis{Source: "unknown", Value: v}, []string{a.attr + " axis skipped"}
	}
}

func (r *reader) axisObject(a *axisPts) (*model.Object, bool) {
	off, ok := r.lower(a.addr)
	if !ok {
		r.unlowered++
		return nil, false
	}
	ms := r.layouts[a.layout]
	mem := findMember(ms, "AXIS_PTS_X")
	if mem.bits == 0 {
		mem = findMember(ms, "FNC_VALUES")
	}
	if mem.bits != 8 && mem.bits != 16 && mem.bits != 32 {
		mem = member{bits: 8}
	}
	_, at := place(ms, map[string]int{"AXIS_PTS_X": a.max}, r.alignW, r.alignL)
	n := a.max
	if n < 1 {
		n = 1
	}
	c := r.comps[a.conv]
	return &model.Object{
		ID: a.name, Shape: "1d", Address: off + model.Addr(at["AXIS_PTS_X"]),
		Rows: 1, Cols: n, Data: dataOf(mem, r.little), Value: valueOf(c, c.prec, nil, nil),
	}, true
}

// lower maps an ECU address to a file offset. ME7 OEM images (and DAMOS) use
// offset = addr - 0x800000. Prefer that over MEMORY_SEGMENT bases: Bosch A2Ls
// list DATA segments such as Dst810000 at 0x810000, and segment-relative
// offsets drop 0x10000 and break axes (R6 on the image).
func (r *reader) lower(addr uint32) (model.Addr, bool) {
	if addr >= 0x800000 && addr < 0x1000000 {
		return model.Addr(addr - 0x800000), true
	}
	for _, s := range r.segs {
		if s.size > 0 && addr >= s.addr && addr < s.addr+s.size {
			return model.Addr(addr - s.addr), true
		}
	}
	if addr < 0x800000 {
		return model.Addr(addr), true
	}
	return 0, false
}

func (r *reader) modCommon(body []token) {
	for i := 0; i < len(body); i++ {
		switch body[i].s {
		case "BYTE_ORDER":
			if i+1 < len(body) {
				r.little = body[i+1].s != "MSB_FIRST"
			}
		case "ALIGNMENT_WORD":
			if i+1 < len(body) {
				r.alignW, _ = strconv.Atoi(body[i+1].s)
			}
		case "ALIGNMENT_LONG":
			if i+1 < len(body) {
				r.alignL, _ = strconv.Atoi(body[i+1].s)
			}
		}
	}
	if r.alignW < 1 {
		r.alignW = 1
	}
	if r.alignL < 1 {
		r.alignL = 1
	}
}

func (r *reader) project(body []token) {
	if len(body) > 0 {
		r.proj = body[0].s
	}
	for _, bl := range nested(body) {
		if bl.name == "HEADER" {
			for i := 0; i < len(bl.body); i++ {
				if bl.body[i].s == "VERSION" && i+1 < len(bl.body) {
					r.ver = unquote(bl.body[i+1])
				}
			}
		}
	}
}

func parseCompu(body []token) (compu, bool) {
	if len(body) < 5 {
		return compu{}, false
	}
	c := compu{name: body[0].s, kind: body[2].s, unit: unquote(body[4]), prec: precision(body[3].s)}
	var coeffs []float64
	for i := 0; i < len(body); i++ {
		if body[i].s == "COEFFS" {
			for j := i + 1; j < len(body) && j < i+7; j++ {
				f, err := strconv.ParseFloat(body[j].s, 64)
				if err != nil {
					break
				}
				coeffs = append(coeffs, f)
			}
		}
	}
	switch c.kind {
	case "IDENTICAL":
		c.c = model.Conversion{Factor: 1}
	case "LINEAR":
		if len(coeffs) >= 2 {
			c.c = model.Conversion{Factor: coeffs[0], Offset: coeffs[1]}
		} else {
			c.c = model.Conversion{Factor: 1}
		}
	case "RAT_FUNC":
		for len(coeffs) < 6 {
			coeffs = append(coeffs, 0)
		}
		c.c, c.warn = ratFunc(coeffs[0], coeffs[1], coeffs[2], coeffs[3], coeffs[4], coeffs[5])
	default:
		c.c = model.Conversion{Factor: 1}
		c.warn = "conversion " + c.kind + " read as X"
	}
	return c, true
}

// ratFunc is ASAM MCD-2 MC RAT_FUNC with COEFFS as internal = f(phys):
//
//	int = (a*phys² + b*phys + c) / (d*phys² + e*phys + f)
//
// The model stores phys = factor*raw + offset (or factor/raw + offset when
// reciprocal). Linear (a = d = e = 0, b and f nonzero) is factor f/b and
// offset -c/b. Reciprocal (a = b = d = f = 0, e nonzero) is factor c/e.
// That matches KP/XDF on Bosch ME7 exports (e.g. COEFFS 0 1 0 0 0 0.01 → 0.01).
func ratFunc(a, b, c, d, e, f float64) (model.Conversion, string) {
	z := func(x float64) bool { return math.Abs(x) < 1e-12 }
	switch {
	case z(a) && z(d) && z(e) && !z(b) && !z(f):
		off := -c / b
		if z(off) {
			off = 0
		}
		return model.Conversion{Factor: f / b, Offset: off}, ""
	case z(a) && z(b) && z(d) && z(f) && !z(e):
		return model.Conversion{Factor: c / e, Reciprocal: true}, ""
	default:
		return model.Conversion{Factor: 1}, "RAT_FUNC is not linear or reciprocal; read as X"
	}
}

func parseLayout(body []token) (string, []member, bool) {
	if len(body) == 0 {
		return "", nil, false
	}
	var ms []member
	for i := 1; i < len(body); i++ {
		k := body[i].s
		if !isLayoutKw(k) || i+2 >= len(body) {
			continue
		}
		pos, err := strconv.Atoi(body[i+1].s)
		if err != nil {
			continue
		}
		bits, signed, flt, ok := dtype(body[i+2].s)
		if !ok {
			continue
		}
		ms = append(ms, member{kind: k, pos: pos, bits: bits, signed: signed, flt: flt})
	}
	// position order
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j].pos < ms[j-1].pos; j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
	return body[0].s, ms, true
}

func isLayoutKw(s string) bool {
	switch s {
	case "NO_AXIS_PTS_X", "NO_AXIS_PTS_Y", "NO_AXIS_PTS_Z",
		"AXIS_PTS_X", "AXIS_PTS_Y", "AXIS_PTS_Z", "FNC_VALUES":
		return true
	}
	return false
}

func dtype(s string) (bits int, signed, flt, ok bool) {
	switch s {
	case "UBYTE":
		return 8, false, false, true
	case "SBYTE":
		return 8, true, false, true
	case "UWORD":
		return 16, false, false, true
	case "SWORD":
		return 16, true, false, true
	case "ULONG":
		return 32, false, false, true
	case "SLONG":
		return 32, true, false, true
	case "FLOAT32_IEEE":
		return 32, false, true, true
	default:
		return 0, false, false, false
	}
}

func parseAxisPts(body []token) (*axisPts, bool) {
	if len(body) < 8 {
		return nil, false
	}
	addr, ok := parseAddr(body[2].s)
	if !ok {
		return nil, false
	}
	max, _ := strconv.Atoi(body[7].s)
	return &axisPts{name: body[0].s, addr: addr, layout: body[4].s, conv: body[6].s, max: max}, true
}

func parseSeg(body []token) (segment, bool) {
	// Name, LongIdentifier, PrgType, MemoryType, Attribute, Address, Size, ...
	if len(body) < 7 {
		return segment{}, false
	}
	addr, ok1 := parseAddr(body[5].s)
	size, ok2 := parseAddr(body[6].s)
	if !ok1 || !ok2 || size == 0 {
		return segment{}, false
	}
	return segment{addr, size}, true
}

func parseFunc(body []token) (function, bool) {
	if len(body) == 0 {
		return function{}, false
	}
	f := function{name: body[0].s}
	for _, bl := range nested(body) {
		if bl.name == "DEF_CHARACTERISTIC" || bl.name == "REF_CHARACTERISTIC" {
			for _, t := range bl.body {
				if t.s != "" && !strings.HasPrefix(t.s, "/") {
					f.chars = append(f.chars, t.s)
				}
			}
		}
	}
	return f, true
}

// place returns the file offset of FNC_VALUES and of each AXIS_PTS_* from the
// start of the record. Counts and points use the member's width. A field
// wider than a byte is aligned to ALIGNMENT_WORD (32-bit to ALIGNMENT_LONG),
// which is the ASAM rule and matches the ME7 one-byte count layout when the
// count fields are UBYTE.
func place(ms []member, pts map[string]int, alignW, alignL int) (int, map[string]int) {
	off := 0
	at := map[string]int{}
	for _, m := range ms {
		sz := m.bits / 8
		if sz < 1 {
			sz = 1
		}
		al := 1
		if sz >= 4 && alignL > 1 {
			al = alignL
		} else if sz >= 2 && alignW > 1 {
			al = alignW
		}
		if al > 1 && off%al != 0 {
			off += al - off%al
		}
		switch {
		case strings.HasPrefix(m.kind, "NO_AXIS_PTS"):
			off += sz
		case strings.HasPrefix(m.kind, "AXIS_PTS"):
			at[m.kind] = off
			n := pts[m.kind]
			if n < 1 {
				n = 1
			}
			off += n * sz
		case m.kind == "FNC_VALUES":
			return off, at
		}
	}
	return off, at
}

func findMember(ms []member, kind string) member {
	for _, m := range ms {
		if m.kind == kind {
			return m
		}
	}
	return member{}
}

func dataOf(m member, little bool) model.Data {
	if m.bits == 0 {
		m.bits = 8
	}
	d := model.Data{Bits: m.bits, Signed: m.signed, Float: m.flt}
	if d.Bits > 8 {
		d.Endian = map[bool]string{true: "little", false: "big"}[little]
	}
	return d
}

func valueOf(c compu, prec int, lo, hi *float64) model.Value {
	if c.c.Factor == 0 && !c.c.Reciprocal {
		c.c.Factor = 1
	}
	u := c.unit
	if u == "-" {
		u = ""
	}
	v := model.Value{Units: u, Precision: prec, Conversion: c.c}
	if lo != nil {
		x := *lo
		v.Min = &x
	}
	if hi != nil {
		x := *hi
		v.Max = &x
	}
	return v
}

func precision(format string) int {
	s := strings.Trim(format, `"`)
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	n := 0
	for _, c := range s[i+1:] {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
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

func add(m map[string]int, k string) map[string]int {
	if m == nil {
		m = map[string]int{}
	}
	m[k]++
	return m
}

type token struct {
	s   string
	str bool
}

func tokenize(b []byte) []token {
	var out []token
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			if i+1 < len(b) {
				i += 2
			} else {
				i = len(b)
			}
		case c == '"':
			i++
			var sb strings.Builder
			for i < len(b) && b[i] != '"' {
				if b[i] == '\\' && i+1 < len(b) {
					i++
				}
				sb.WriteRune(rune(b[i]))
				i++
			}
			if i < len(b) {
				i++
			}
			out = append(out, token{sb.String(), true})
		default:
			j := i
			for j < len(b) && b[j] > ' ' && b[j] != '"' {
				j++
			}
			out = append(out, token{latin1(b[i:j]), false})
			i = j
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

func unquote(t token) string {
	s := strings.TrimSpace(t.s)
	if s == `""` {
		return ""
	}
	return s
}

// cleanText drops tabs from A2L display text (Bosch often uses TAB after a colon).
func cleanText(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\t", " "))
}

func has(toks []token, s string) bool {
	for _, t := range toks {
		if t.s == s {
			return true
		}
	}
	return false
}

// allBlocks returns every block, including those nested in another.
func allBlocks(toks []token) []block {
	var out []block
	for _, bl := range nested(toks) {
		out = append(out, bl)
		out = append(out, allBlocks(bl.body)...)
	}
	return out
}

func nested(toks []token) []block {
	var out []block
	for i := 0; i < len(toks); i++ {
		if toks[i].s != "/begin" || i+1 >= len(toks) {
			continue
		}
		name := toks[i+1].s
		inner, end := untilEnd(toks, i)
		out = append(out, block{name, inner})
		i = end
	}
	return out
}

// untilEnd returns the tokens inside the block that starts at /begin (index i)
// and the index of its /end token.
func untilEnd(toks []token, i int) ([]token, int) {
	depth := 0
	start := i + 2
	for j := i; j < len(toks); j++ {
		switch toks[j].s {
		case "/begin":
			depth++
		case "/end":
			depth--
			if depth == 0 {
				if start > j {
					start = j
				}
				return toks[start:j], j
			}
		}
	}
	return toks[start:], len(toks) - 1
}

func parseAddr(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	base := 10
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		base = 16
		s = s[2:]
	} else if strings.HasPrefix(s, "$") {
		base = 16
		s = s[1:]
	}
	if s == "" || strings.ContainsAny(s, ".eE") && base == 10 {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || f < 0 || f > math.MaxUint32 {
			return 0, false
		}
		return uint32(f), true
	}
	n, err := strconv.ParseUint(s, base, 32)
	return uint32(n), err == nil
}
