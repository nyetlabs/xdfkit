package kp

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	ols5KP   = "../testdata/local/8N0906018BC_0002_366366.kp"
	ols5JSON = "../testdata/local/8N0906018BB-0003.json"
	ols5A2L  = "../testdata/local/8N0906018BB_0002.a2l"
)

func TestOLS5Parse(t *testing.T) {
	b, err := os.ReadFile(ols5KP)
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Layout() != OLS5 {
		t.Fatalf("layout %v, want OLS5", f.Layout())
	}
	if n := len(f.Project.Maps); n < 6000 {
		t.Fatalf("maps %d, want thousands", n)
	}
	if n := len(f.Project.Folders); n < 100 {
		t.Fatalf("folders %d", n)
	}
	if _, err := f.Encode(); err == nil {
		t.Fatal("Encode of OLS5 should fail")
	}
}

// TestOLS5JSONIntersection treats BB-0003.json as a secondary oracle: it came
// from a third-party XDF (provenance origin damos) and was lint-fixed, so it is
// not authoritative alone. Against KP+A2L it still agrees on address and
// factor (XDF decimal rounding only); use A2L when they conflict.
func TestOLS5JSONIntersection(t *testing.T) {
	kb, err := os.ReadFile(ols5KP)
	if err != nil {
		t.Skip(err)
	}
	jb, err := os.ReadFile(ols5JSON)
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(kb)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Objects []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
			Cols    int    `json:"cols"`
			Rows    int    `json:"rows"`
			Value   struct {
				Conversion struct {
					Factor float64 `json:"factor"`
				} `json:"conversion"`
			} `json:"value"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(jb, &doc); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*Map, len(f.Project.Maps))
	for _, m := range f.Project.Maps {
		byID[m.ID] = m
	}
	var match, checked, facRound, shapeDiff int
	for _, o := range doc.Objects {
		m := byID[o.ID]
		if m == nil {
			continue
		}
		checked++
		addr, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(o.Address), "0x"), 16, 64)
		if err != nil {
			t.Fatalf("%s: %v", o.ID, err)
		}
		off := uint32(addr)
		if addr >= 0x800000 {
			off = uint32(addr - 0x800000)
		}
		if m.Start != off {
			continue // BB vs BC drift
		}
		jf := o.Value.Conversion.Factor
		if jf != 0 && !almostEqual(m.Value.Factor, jf) {
			// WinOLS DAMOS→XDF often writes reciprocal conversions as factor 1.
			if jf == 1 && (m.Reciprocal || !almostEqual(m.Value.Factor, 1)) {
				match++
				continue
			}
			// XDF path rounds binary fractions (0.0078125 → 0.007813); allow ~2%.
			rel := math.Abs(m.Value.Factor-jf) / math.Max(math.Abs(m.Value.Factor), 1e-30)
			if rel > 0.02 {
				t.Errorf("%s factor kp=%g json=%g rel=%.3g", o.ID, m.Value.Factor, jf, rel)
				continue
			}
			facRound++
		}
		if o.Cols > 0 {
			same := int(m.Cols) == o.Cols && int(m.Rows) == o.Rows
			swap := int(m.Cols) == o.Rows && int(m.Rows) == o.Cols
			if !same && !swap {
				shapeDiff++
			}
		}
		match++
	}
	if checked < 5000 || match < 5000 {
		t.Fatalf("checked=%d match=%d (addr+factor)", checked, match)
	}
	if shapeDiff > 5 {
		t.Fatalf("shape diffs %d (expected a couple)", shapeDiff)
	}
	t.Logf("addr-matched=%d factorXdfRounding=%d shapeDiff=%d", match, facRound, shapeDiff)
}

// TestOLS5A2LIntersection checks the KP against a WinOLS 5 A2L export of the
// related BB project. File offsets are ECU address minus 0x800000; BB↔BC
// address drift is skipped. Linear RAT_FUNC in this export matches KP as f/b.
func TestOLS5A2LIntersection(t *testing.T) {
	kb, err := os.ReadFile(ols5KP)
	if err != nil {
		t.Skip(err)
	}
	ab, err := os.ReadFile(ols5A2L)
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(kb)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]*Map, len(f.Project.Maps))
	for _, m := range f.Project.Maps {
		byID[m.ID] = m
	}
	text := string(ab)
	compu := map[string]float64{}
	for _, m := range reA2LCompu.FindAllStringSubmatch(text, -1) {
		c := reA2LCoeffs.FindStringSubmatch(m[2])
		if c == nil {
			continue
		}
		var a, b, c0, d, e, f0 float64
		if _, err := fmt.Sscanf(strings.Join(c[1:], " "), "%f %f %f %f %f %f", &a, &b, &c0, &d, &e, &f0); err != nil {
			continue
		}
		if a == 0 && d == 0 && e == 0 && b != 0 {
			compu[m[1]] = f0 / b
		}
	}
	var match, checked, facDiff int
	for _, m := range reA2LChar.FindAllStringSubmatch(text, -1) {
		id, addrS, conv := m[1], m[2], m[3]
		km := byID[id]
		if km == nil {
			continue
		}
		checked++
		addr, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(addrS), "0x"), 16, 64)
		if err != nil {
			t.Fatal(id, err)
		}
		off := uint32(addr)
		if addr >= 0x800000 {
			off = uint32(addr - 0x800000)
		}
		if km.Start != off {
			continue
		}
		if fac, ok := compu[conv]; ok {
			if !almostEqual(km.Value.Factor, fac) {
				t.Errorf("%s factor kp=%g a2l=%g conv=%s", id, km.Value.Factor, fac, conv)
				facDiff++
				continue
			}
		}
		match++
	}
	if checked < 5000 || match < 4500 || facDiff != 0 {
		t.Fatalf("checked=%d match=%d facDiff=%d", checked, match, facDiff)
	}
}

func almostEqual(a, b float64) bool {
	d := math.Abs(a - b)
	return d <= 1e-9 || d <= 1e-6*math.Max(math.Abs(a), math.Abs(b))
}

var (
	reA2LCompu  = regexp.MustCompile(`(?s)/begin\s+COMPU_METHOD\s+(\S+)\s+".*?"\s+\S+\s+"[^"]*"\s+"[^"]*"(.*?)/end\s+COMPU_METHOD`)
	reA2LCoeffs = regexp.MustCompile(`COEFFS\s+([-\d.eE+]+)\s+([-\d.eE+]+)\s+([-\d.eE+]+)\s+([-\d.eE+]+)\s+([-\d.eE+]+)\s+([-\d.eE+]+)`)
	reA2LChar   = regexp.MustCompile(`(?s)/begin\s+CHARACTERISTIC\s+(\S+)\s+".*?"\s+\S+\s+(0x[0-9A-Fa-f]+)\s+\S+\s+\S+\s+(\S+)\s`)
)
