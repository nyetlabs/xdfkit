// Package lint checks a KP file's image axes against the flash image, and its
// map end addresses, and proposes fixes (docs/autocorrect.md). Findings are
// grouped by address, so an axis shared by several maps is reported once.
// Apply makes the proposed changes to a kp.File; only fixed-width fields that
// are already decoded change.
package lint

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.nyet.org/xdfkit/kp"
)

// Families: how an ECU stores axis breakpoints.
const (
	ME7 = "me7" // absolute values, preceded by a count byte (ME7.x)
	M3  = "m3"  // "EEPROM, subtract" offsets, preceded by [variable id][count] (Motronic M3.x, M5.x)
)

// Confidence levels, highest first.
const (
	High   = "high"
	Medium = "medium"
	Low    = "low"
	None   = "n/a" // report only
)

var confRank = map[string]int{High: 3, Medium: 2, Low: 1, None: 0}

// AtLeast reports whether confidence c meets the minimum min.
func AtLeast(c, min string) bool { return confRank[c] >= confRank[min] }

// AxisRef names one map axis that uses a finding's axis.
type AxisRef struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Axis  string `json:"axis,omitempty"` // x or y; empty for map findings
}

// Fix is a proposed change; nil fields are left alone.
type Fix struct {
	DataSource *kp.DataSource `json:"dataSource,omitempty"`
	Signed     *bool          `json:"signed,omitempty"`
	End        *uint32        `json:"end,omitempty"`
}

// Finding is one problem with one axis (address and point count) or one map
// (start address and byte length, rule R8), with the evidence and the
// proposed fix.
type Finding struct {
	ID         string         `json:"id"` // RULE@0xADDR:POINTS
	Rule       string         `json:"rule"`
	Confidence string         `json:"confidence"`
	Message    string         `json:"message"`
	Addr       uint32         `json:"addr"`
	Points     int            `json:"points"` // bytes for map findings
	DataSource *kp.DataSource `json:"dataSource,omitempty"`
	Type       kp.Type        `json:"type"`
	Signed     bool           `json:"signed"`
	Values     []int64        `json:"values,omitempty"`     // raw, as stored
	Subtracted []int64        `json:"subtracted,omitempty"` // Values as WinOLS shows a "subtract" axis
	Opposite   []int64        `json:"opposite,omitempty"`   // raw, with the other signedness
	CountByte  *int           `json:"countByte,omitempty"`
	End        *uint32        `json:"end,omitempty"` // stored map end address
	Fix        *Fix           `json:"fix,omitempty"`
	Maps       []AxisRef      `json:"maps"`
}

// Options select the family; empty means detect it from the image.
type Options struct {
	Family string `json:"family,omitempty"`
}

// Report is the result of a lint run.
type Report struct {
	Family       string    `json:"family"`
	FamilySource string    `json:"familySource"` // "ident ME7.1", or "option"
	Findings     []Finding `json:"findings"`
}

// identRe matches Bosch identification strings such as
// 40/1/ME7.1/5/6005.01//22m/DstC2o/011200/ and 5655/1/M3.82/03/400600/DAMOS3N2/....
var identRe = regexp.MustCompile(`[0-9]{1,4}/[0-9]/(M[A-Z0-9.]+)/[0-9]{1,3}/[!-~]*/`)

// Family names the axis storage family of an image from its Bosch
// identification string, and the evidence.
func Family(img []byte) (string, string, error) {
	m := identRe.FindSubmatch(img)
	if m == nil {
		return "", "", fmt.Errorf("no Bosch identification string in the image; give the family (%s or %s)", ME7, M3)
	}
	id := string(m[1])
	switch {
	case strings.HasPrefix(id, "ME7"):
		return ME7, "ident " + id, nil
	case strings.HasPrefix(id, "M3.") || strings.HasPrefix(id, "M5."):
		return M3, "ident " + id, nil
	}
	return "", "", fmt.Errorf("unknown ECU family %s; give the family (%s or %s)", id, ME7, M3)
}

// Lint checks every defined image axis of every map.
func Lint(f *kp.File, img []byte, opt Options) (Report, error) {
	r := Report{Family: opt.Family, FamilySource: "option"}
	if r.Family == "" {
		var err error
		if r.Family, r.FamilySource, err = Family(img); err != nil {
			return r, err
		}
	} else if r.Family != ME7 && r.Family != M3 {
		return r, fmt.Errorf("unknown family %q (want %s or %s)", r.Family, ME7, M3)
	}
	byID := map[string]int{}
	add := func(fd *Finding, m *kp.Map, axis string) {
		if fd == nil {
			return
		}
		ref := AxisRef{Index: m.Index, ID: m.ID, Name: m.Name, Axis: axis}
		if i, ok := byID[fd.ID]; ok {
			r.Findings[i].Maps = append(r.Findings[i].Maps, ref)
			return
		}
		fd.Maps = []AxisRef{ref}
		byID[fd.ID] = len(r.Findings)
		r.Findings = append(r.Findings, *fd)
	}
	for _, m := range f.Project.Maps {
		add(checkEnd(m), m, "")
		add(check(m.X, int(m.Cols), img, r.Family), m, "x")
		add(check(m.Y, int(m.Rows), img, r.Family), m, "y")
	}
	sort.Slice(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		return a.Addr < b.Addr || a.Addr == b.Addr && a.ID < b.ID
	})
	return r, nil
}

// check applies the rules to one axis and returns its finding, if any.
func check(a *kp.Axis, n int, img []byte, family string) *Finding {
	if a == nil || !a.Defined || !a.DataSource.FromImage() || n < 2 || a.Type.Width() == 0 || a.Type.Float() {
		return nil
	}
	w := a.Type.Width()
	v := read(img, a.Addr, n, a.Type, a.Signed)
	if v == nil {
		return nil
	}
	fd := &Finding{
		Points: n, Addr: a.Addr, DataSource: &a.DataSource, Type: a.Type, Signed: a.Signed,
		Values: v, Subtracted: subtracted(v, w),
	}
	if w == 1 && a.Addr >= 1 {
		c := int(img[a.Addr-1])
		fd.CountByte = &c
	}
	opp := read(img, a.Addr, n, a.Type, !a.Signed)
	delta := a.DataSource != kp.DSEeprom
	set := func(rule, conf, msg string, fix *Fix) *Finding {
		fd.ID = fmt.Sprintf("%s@0x%x:%d", rule, a.Addr, n)
		fd.Rule, fd.Confidence, fd.Message, fd.Fix = rule, conf, msg, fix
		return fd
	}
	if allEqual(v) {
		return set("R6", None, "all values identical: wrong address or size", nil)
	}
	if family == ME7 {
		switch {
		case delta && monotonic(v):
			return set("R1", High, fmt.Sprintf("%s axis, but ME7 axes are absolute and the plain values are monotonic", a.DataSource), &Fix{DataSource: ptr(kp.DSEeprom)})
		case delta && monotonic(opp):
			fd.Opposite = opp
			return set("R1", Medium, fmt.Sprintf("%s axis, but ME7 axes are absolute; monotonic only with the opposite signedness", a.DataSource), &Fix{DataSource: ptr(kp.DSEeprom), Signed: ptr(!a.Signed)})
		case delta:
			return set("R6", None, fmt.Sprintf("%s axis that is not monotonic plain either: check address, size or type", a.DataSource), nil)
		case !monotonic(v) && monotonic(opp):
			fd.Opposite = opp
			return set("R3", Medium, "monotonic only with the opposite signedness", &Fix{Signed: ptr(!a.Signed)})
		case !monotonic(v):
			return set("R6", None, "not monotonic: check address, size or type", nil)
		}
		return nil
	}
	sub := increasing(fd.Subtracted) && fd.Subtracted[0] >= 0
	switch {
	case !delta && !monotonic(v) && sub:
		return set("R2", High, "plain axis in an M3.x image that is increasing only read as \"EEPROM, subtract\"", &Fix{DataSource: ptr(kp.DSEepromSubtract)})
	case fd.CountByte != nil && *fd.CountByte != n:
		return set("R4", None, fmt.Sprintf("count byte before the axis is %d, axis size is %d: check address or size", *fd.CountByte, n), nil)
	case delta && !sub:
		return set("R6", None, fmt.Sprintf("%s axis that is not increasing from 0 or more when read as \"EEPROM, subtract\": check address, size or type", a.DataSource), nil)
	}
	return nil
}

// subtracted returns the points WinOLS 2.24 displays for an "EEPROM,
// subtract" axis of w-byte values: point i = 2^(8w) - (v[i] + ... + v[n-1]).
func subtracted(v []int64, w int) []int64 {
	s := make([]int64, len(v))
	t := int64(1) << (8 * w)
	for i := len(v) - 1; i >= 0; i-- {
		t -= v[i]
		s[i] = t
	}
	return s
}

// mapBytes is the byte length of a map's data, or 0 if unknown.
func mapBytes(m *kp.Map) int { return int(m.Cols) * int(m.Rows) * m.Type.Width() }

// checkEnd applies rule R8: WinOLS writes a map's end address as start +
// byte length and recomputes it on export, so a different value is stale.
func checkEnd(m *kp.Map) *Finding {
	n := mapBytes(m)
	end := m.Start + uint32(n)
	if n <= 0 || m.End == end {
		return nil
	}
	return &Finding{
		ID: fmt.Sprintf("R8@0x%x:%d", m.Start, n), Rule: "R8", Confidence: High,
		Message: fmt.Sprintf("end address 0x%x, but start + %d bytes is 0x%x", m.End, n, end),
		Addr:    m.Start, Points: n, Type: m.Type, Signed: m.Signed, End: ptr(m.End),
		Fix: &Fix{End: &end},
	}
}

func ptr[T any](v T) *T { return &v }

// read returns n raw values at addr, or nil if they don't fit in the image.
func read(img []byte, addr uint32, n int, t kp.Type, signed bool) []int64 {
	w := t.Width()
	end := uint64(addr) + uint64(n*w)
	if end > uint64(len(img)) {
		return nil
	}
	v := make([]int64, n)
	for i := range v {
		b := img[int(addr)+i*w : int(addr)+(i+1)*w]
		var u uint64
		for j := range w {
			k := j
			if !t.LE() {
				k = w - 1 - j
			}
			u |= uint64(b[k]) << (8 * j)
		}
		if signed && u&(1<<(8*w-1)) != 0 {
			v[i] = int64(u) - int64(1)<<(8*w)
		} else {
			v[i] = int64(u)
		}
	}
	return v
}

func allEqual(v []int64) bool {
	for _, x := range v[1:] {
		if x != v[0] {
			return false
		}
	}
	return true
}

func increasing(v []int64) bool {
	for i := 1; i < len(v); i++ {
		if v[i] <= v[i-1] {
			return false
		}
	}
	return true
}

func decreasing(v []int64) bool {
	for i := 1; i < len(v); i++ {
		if v[i] >= v[i-1] {
			return false
		}
	}
	return true
}

func monotonic(v []int64) bool { return v != nil && (increasing(v) || decreasing(v)) }

// Select returns the findings to apply: those named in only (if any), else
// those whose rule is in rules (if any) with at least confidence min.
func Select(fs []Finding, rules, only []string, min string) []Finding {
	var out []Finding
	for _, f := range fs {
		switch {
		case f.Fix == nil:
		case len(only) > 0:
			if contains(only, f.ID) {
				out = append(out, f)
			}
		case (len(rules) == 0 || contains(rules, f.Rule)) && AtLeast(f.Confidence, min):
			out = append(out, f)
		}
	}
	return out
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// Apply makes each finding's fix on every map or map axis it names. It fails
// if one no longer has the finding's address and size, so a stale findings
// file can't change the wrong map.
func Apply(f *kp.File, fs []Finding) error {
	byIndex := map[int]*kp.Map{}
	for _, m := range f.Project.Maps {
		byIndex[m.Index] = m
	}
	for _, fd := range fs {
		if fd.Fix == nil {
			return fmt.Errorf("%s: no fix to apply", fd.ID)
		}
		for _, ref := range fd.Maps {
			m := byIndex[ref.Index]
			if m == nil {
				return fmt.Errorf("%s: no map with index %d", fd.ID, ref.Index)
			}
			if ref.Axis == "" {
				if m.Start != fd.Addr || mapBytes(m) != fd.Points {
					return fmt.Errorf("%s: map %d (%s) no longer matches", fd.ID, ref.Index, ref.ID)
				}
				if fd.Fix.End != nil {
					m.End = *fd.Fix.End
				}
				continue
			}
			a, n := m.X, m.Cols
			if ref.Axis == "y" {
				a, n = m.Y, m.Rows
			}
			if a == nil || a.Addr != fd.Addr || int(n) != fd.Points {
				return fmt.Errorf("%s: map %d (%s) %s axis no longer matches", fd.ID, ref.Index, ref.ID, ref.Axis)
			}
			if fd.Fix.DataSource != nil {
				a.DataSource = *fd.Fix.DataSource
			}
			if fd.Fix.Signed != nil {
				a.Signed = *fd.Fix.Signed
			}
		}
	}
	return nil
}
