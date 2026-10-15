package lint

import (
	"fmt"
	"slices"
	"sort"

	"go.nyet.org/xdfkit/kp"
)

// maxPoints caps an ME7 axis's points when searching for a record.
const maxPoints = 32

// Record is an ME7 inline record as the image holds it (docs/model.md, ME7
// record layout): one count byte per axis, the axes in that order, then the
// values, with every 16-bit field at an even address.
type Record struct {
	Start  uint32       `json:"start,omitempty"` // first count; 0 if not read
	Counts []int        `json:"counts,omitempty"`
	Axes   []RecordAxis `json:"axes"`
	Values uint32       `json:"values,omitempty"` // the axis for an axis-point table; 0 if not read
}

// RecordAxis is one axis of a record.
type RecordAxis struct {
	Addr uint32 `json:"addr"`
	Bits int    `json:"bits"`
}

// Causes: how a def differs from the record the image holds (R9, R10).
const (
	ValuesPad   = "values-pad"   // values right after the axes: the pad before them was missed
	TableLate   = "table-late"   // axis-point table one byte after its first point
	AxisPad     = "axis-pad"     // 16-bit axis one byte off: a pad missed, or read as the axis
	AxisWidth   = "axis-width"   // 8-bit axis read as 16-bit
	Shape       = "shape"        // map next to a record of another shape
	RecordStart = "record-start" // R10: map starts on the record's first count
)

var causeText = map[string]string{
	ValuesPad:   "the values pad after the axes was missed",
	TableLate:   "an axis-point table read one byte late",
	AxisPad:     "a pad next to the axis was missed or read",
	AxisWidth:   "an 8-bit axis read as 16-bit",
	Shape:       "a record of another shape",
	RecordStart: "the map starts on the record's count",
}

// describe names a finding's cause and the record behind it.
func describe(cause string, r *Record) string {
	if r == nil {
		return "no record found"
	}
	s := ""
	if r.Start != 0 {
		s = fmt.Sprintf(" at 0x%x, counts %v,", r.Start, r.Counts)
	}
	for i, a := range r.Axes {
		if i > 0 {
			s += ","
		}
		s += fmt.Sprintf(" %d-bit axis at 0x%x", a.Bits, a.Addr)
	}
	if r.Values != 0 {
		s += fmt.Sprintf(", values at 0x%x", r.Values)
	}
	if cause == "" {
		return "record" + s
	}
	return fmt.Sprintf("%s: %s; record%s", cause, causeText[cause], s)
}

// even rounds a up to an even address for a field of w bytes.
func even(a uint32, w int) uint32 {
	if w > 1 && a%2 == 1 {
		return a + 1
	}
	return a
}

// intType is the little-endian integer type of w bytes (1 or 2).
func intType(w int) kp.Type { return kp.Type(2*w - 1) } // u8, 16-lohi

// axisOK reports whether n w-byte points at a are monotonic, read unsigned
// or signed.
func axisOK(img []byte, a uint32, n, w int) bool {
	t := intType(w)
	return monotonic(read(img, a, n, t, false)) || monotonic(read(img, a, n, t, true))
}

// inline reads the record whose counts start at start, with axes of the
// given widths in bytes, in order, and values of w bytes. Every count must be
// 2 to maxPoints and every axis monotonic.
func inline(img []byte, start uint32, widths []int, w int) (Record, bool) {
	r := Record{Start: start}
	a := start + uint32(len(widths))
	if uint64(a) > uint64(len(img)) {
		return r, false
	}
	for i, aw := range widths {
		n := int(img[start+uint32(i)])
		a = even(a, aw)
		if n < 2 || n > maxPoints || !axisOK(img, a, n, aw) {
			return r, false
		}
		r.Counts = append(r.Counts, n)
		r.Axes = append(r.Axes, RecordAxis{a, 8 * aw})
		a += uint32(n * aw)
	}
	r.Values = even(a, w)
	return r, true
}

// table reads an axis-point table of n w-byte points whose count is at
// start: the axis is the values.
func table(img []byte, start uint32, n, w int) (Record, bool) {
	r, ok := inline(img, start, []int{w}, w)
	if !ok || r.Counts[0] != n {
		return r, false
	}
	r.Values = r.Axes[0].Addr
	return r, true
}

var layouts = [][]int{{1}, {2}, {1, 1}, {1, 2}, {2, 1}, {2, 2}}

// around finds the one curve or map record whose values start one byte
// either side of s, ignoring records inside another candidate.
func around(img []byte, s uint32, w int) (Record, bool) {
	var hit []Record
	for h := s - min(s, 3+4*maxPoints+2); h < s; h++ {
		for _, l := range layouts {
			if r, ok := inline(img, h, l, w); ok && (r.Values == s+1 || r.Values == s-1) {
				hit = append(hit, r)
			}
		}
	}
	var out []Record
	for i, r := range hit {
		inside := false
		for j, o := range hit {
			if j != i && o.Start <= r.Start && r.Values <= o.Values && (o.Start < r.Start || r.Values < o.Values) {
				inside = true
			}
		}
		if !inside {
			out = append(out, r)
		}
	}
	if len(out) != 1 {
		return Record{}, false
	}
	return out[0], true
}

// sameCounts reports whether a and b hold the same counts in any order.
func sameCounts(a, b []int) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// imageAxis reports whether a is an axis of n integer points read from the
// image.
func imageAxis(a *kp.Axis, n int) bool {
	return a != nil && a.Defined && a.DataSource.FromImage() && n >= 2 && a.Type.Width() > 0 && !a.Type.Float()
}

// mapCounts are a map's point counts, one per axis.
func mapCounts(m *kp.Map) []int {
	if m.Rows > 1 {
		return []int{int(m.Rows), int(m.Cols)}
	}
	return []int{int(m.Cols)}
}

// mapRecord explains a map of w-byte cells at an odd address from the record
// the image holds around it.
func mapRecord(m *kp.Map, img []byte) (string, *Record) {
	s, w := m.Start, m.Type.Width()
	type span struct {
		a    uint32
		n, w int
	}
	var spans []span
	if imageAxis(m.X, int(m.Cols)) {
		spans = append(spans, span{m.X.Addr, int(m.Cols), m.X.Type.Width()})
	}
	if m.Rows > 1 && imageAxis(m.Y, int(m.Rows)) {
		spans = append(spans, span{m.Y.Addr, int(m.Rows), m.Y.Type.Width()})
	}
	if len(spans) > 0 {
		sort.Slice(spans, func(i, j int) bool { return spans[i].a < spans[j].a })
		last := spans[len(spans)-1]
		a0, k := spans[0].a, uint32(len(spans))
		if last.a+uint32(last.n*last.w) != s || a0 < k+1 {
			return "", nil
		}
		var widths, sizes []int
		for _, p := range spans {
			widths, sizes = append(widths, p.w), append(sizes, p.n)
		}
		starts := []uint32{a0 - k, a0 - k - 1}
		if starts[0]%2 == 1 {
			starts[0], starts[1] = starts[1], starts[0]
		}
		for _, st := range starts {
			r, ok := inline(img, st, widths, w)
			if ok && r.Values == s+1 && r.Axes[0].Addr == a0 && sameCounts(r.Counts, sizes) {
				return ValuesPad, &r
			}
		}
		return "", nil
	}
	n := int(m.Cols) * int(m.Rows)
	if m.Rows <= 1 && s >= 3 {
		if r, ok := table(img, s-3, n, w); ok && r.Values == s-1 {
			return TableLate, &r
		}
	}
	r, ok := around(img, s, w)
	switch {
	case !ok:
		return "", nil
	case !sameCounts(r.Counts, mapCounts(m)):
		return Shape, &r
	case r.Values == s+1:
		return ValuesPad, &r
	}
	return "", &r
}

// axisRecord explains a 16-bit axis of n points at an odd address; other
// is the map's other axis, of on points.
func axisRecord(a *kp.Axis, n int, other *kp.Axis, on int, img []byte) (string, *Record) {
	at := a.Addr
	end := uint32(0)
	if imageAxis(other, on) {
		end = other.Addr + uint32(on*other.Type.Width())
	}
	at16 := func(start, addr uint32, cause string) (string, *Record) {
		r := &Record{Start: start, Axes: []RecordAxis{{addr, 16}}}
		if start != 0 {
			r.Counts = []int{n}
		}
		return cause, r
	}
	at8 := func(addr uint32) (string, *Record) {
		return AxisWidth, &Record{Start: addr - 1, Counts: []int{n}, Axes: []RecordAxis{{addr, 8}}}
	}
	if at < 3 || int(at)+1 >= len(img) {
		return "", nil
	}
	cnt := func(addr uint32) bool { return int(img[addr]) == n }
	switch {
	case end == at && axisOK(img, at+1, n, 2):
		return at16(0, at+1, AxisPad)
	case cnt(at-1) && axisOK(img, at, n, 1):
		return at8(at)
	case cnt(at-2) && axisOK(img, at-1, n, 1):
		return at8(at - 1)
	case cnt(at-1) && img[at] == 0 && axisOK(img, at+1, n, 2):
		return at16(at-1, at+1, AxisPad)
	case cnt(at-3) && img[at-2] == 0 && axisOK(img, at-1, n, 2):
		return at16(at-3, at-1, AxisPad)
	case end != 0 && end == at-2 && axisOK(img, at-1, n, 2):
		return at16(0, at-1, AxisPad)
	}
	return "", nil
}
