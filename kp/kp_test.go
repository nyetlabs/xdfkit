package kp

import (
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// mapdump's legend for the CSV "Organization" column, which is really the value type.
var mapdumpType = map[Type]string{
	1: "8 Bit", 2: "16 Bit (HiLo)", 3: "16 Bit (LoHi)", 4: "32 Bit (HiLoHilo)",
	5: "32 Bit (LoHiLoHi)", 6: "32 BitFloat (HiLoHiLo)", 7: "32 BitFloat (LoHiLoHi)",
}

// missing skips the test, or fails it when XDFKIT_REQUIRE_DATA is set (CI), so
// absent test inputs (ecuxplot data, jq) can't turn a run silently green.
func missing(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("XDFKIT_REQUIRE_DATA") != "" {
		t.Fatalf(format+" (XDFKIT_REQUIRE_DATA is set)", args...)
	}
	t.Skipf(format, args...)
}

func dataDir(t *testing.T) string {
	d := os.Getenv("XDFKIT_ECUXPLOT_DATA")
	if d == "" {
		d = filepath.Join("..", "..", "ecuxplot", "data")
	}
	if _, err := os.Stat(d); err != nil {
		missing(t, "ecuxplot data not found at %s (set XDFKIT_ECUXPLOT_DATA)", d)
	}
	return d
}

func axisAddr(a *Axis) string {
	if !a.DataSource.FromImage() {
		return "-"
	}
	return fmt.Sprintf("0x%x", a.Addr)
}

// dash mirrors mapdump's CSV writer, which prints empty strings as "-".
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// row renders the CSV columns this test compares: ID, Address, Name, Size, type, X address, Y address.
func row(m *Map) string {
	return strings.Join([]string{dash(m.ID), fmt.Sprintf("0x%x", m.Start), dash(m.Name),
		fmt.Sprintf("%dx%d", m.Cols, m.Rows), mapdumpType[m.Type], axisAddr(m.X), axisAddr(m.Y)}, "|")
}

// TestMatchesMapdump compares every map against mapdump's CSV output in ecuxplot/data.
func TestMatchesMapdump(t *testing.T) {
	dir := dataDir(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, kpPath := range kps {
		csvPath := strings.TrimSuffix(kpPath, ".kp") + ".csv"
		if _, err := os.Stat(csvPath); err != nil {
			continue
		}
		t.Run(filepath.Base(kpPath), func(t *testing.T) {
			data, err := os.ReadFile(kpPath)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			maps := append([]*Map(nil), f.Project.Maps...)
			sort.SliceStable(maps, func(i, j int) bool { return maps[i].Start < maps[j].Start })

			cf, err := os.Open(csvPath)
			if err != nil {
				t.Fatal(err)
			}
			defer cf.Close()
			cr := csv.NewReader(cf)
			cr.FieldsPerRecord = -1
			cr.LazyQuotes = true
			recs, err := cr.ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			recs = recs[1:]
			if len(recs) != len(maps) {
				t.Fatalf("map count: kp %d, csv %d", len(maps), len(recs))
			}
			bad, quoted := 0, 0
			for i, r := range recs {
				// mapdump's CSV writer does not escape quotes, so those rows are malformed.
				if strings.Contains(maps[i].Name, `"`) {
					quoted++
					continue
				}
				want := strings.Join([]string{r[0], r[1], r[2], r[3], r[4], r[7], r[8]}, "|")
				if got := row(maps[i]); got != want && bad < 5 {
					bad++
					t.Errorf("map %d:\n got  %s\n want %s", i, got, want)
				}
			}
			t.Logf("%d maps, %d skipped (quotes in name)", len(maps), quoted)
		})
	}
}

// TestRoundTrip re-encodes every pack twice: straight from Parse, which must be
// byte-identical, and via JSON, which re-deflates v2 map blocks. A re-deflated
// pack must match outside the zip (apart from the file length at 0x14) and
// inflate to the same map block.
func TestRoundTrip(t *testing.T) {
	dir := dataDir(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, kpPath := range kps {
		t.Run(filepath.Base(kpPath), func(t *testing.T) {
			data, err := os.ReadFile(kpPath)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			if f.Layout() == V2 && int(f.HeaderLen) != len(data) {
				t.Errorf("header length %d, file length %d", f.HeaderLen, len(data))
			}
			out, err := f.Encode()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out, data) {
				t.Errorf("Encode(Parse) differs at 0x%x", diffAt(out, data))
			}

			js, err := json.Marshal(f)
			if err != nil {
				t.Fatal(err)
			}
			var g File
			if err := json.Unmarshal(js, &g); err != nil {
				t.Fatal(err)
			}
			if out, err = g.Encode(); err != nil {
				t.Fatal(err)
			}
			if f.Layout() == V1 {
				if !bytes.Equal(out, data) {
					t.Errorf("via JSON differs at 0x%x", diffAt(out, data))
				}
				return
			}
			h, err := Parse(out)
			if err != nil {
				t.Fatal(err)
			}
			if int(h.HeaderLen) != len(out) {
				t.Errorf("via JSON: header length %d, file length %d", h.HeaderLen, len(out))
			}
			if !bytes.Equal(h.Project.intern, f.Project.intern) {
				t.Errorf("via JSON: map block differs at 0x%x", diffAt(h.Project.intern, f.Project.intern))
			}
			patched := append([]byte(nil), out[:h.Project.zipOff]...)
			copy(patched[headerLenOff:], data[headerLenOff:headerLenOff+4])
			if !bytes.Equal(patched, data[:f.Project.zipOff]) {
				t.Errorf("via JSON: header differs at 0x%x", diffAt(patched, data[:f.Project.zipOff]))
			}
			tail := func(b []byte, x *File) []byte { return b[x.Project.zipOff+4+len(x.Project.zip):] }
			if !bytes.Equal(tail(out, h), tail(data, f)) {
				t.Errorf("via JSON: data after the zip differs")
			}
			t.Logf("zip %d bytes, re-deflated %d", len(f.Project.zip), len(h.Project.zip))
		})
	}
}

// TestEdit changes a string length (shifting everything after it) and an address,
// then checks the re-encoded pack parses back with the edits and nothing else changed.
func TestEdit(t *testing.T) {
	dir := dataDir(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, kpPath := range kps {
		t.Run(filepath.Base(kpPath), func(t *testing.T) {
			data, err := os.ReadFile(kpPath)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := Parse(data)
			m, w := f.Project.Maps[0], want.Project.Maps[0]
			m.Name += " (edited €)"
			m.Start += 2
			w.Name, w.Start = m.Name, m.Start
			out, err := f.Encode()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(out)
			if err != nil {
				t.Fatal(err)
			}
			if f.Layout() == V2 {
				want.HeaderLen = int32(len(out))
			}
			gj, _ := json.Marshal(got)
			wj, _ := json.Marshal(want)
			if !bytes.Equal(gj, wj) {
				t.Errorf("re-parsed pack differs from the edit")
			}
		})
	}
}

// TestOffset reads the patchable fields of a few maps back at the offsets
// Offsets reports.
func TestOffset(t *testing.T) {
	dir := dataDir(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, kpPath := range kps {
		t.Run(filepath.Base(kpPath), func(t *testing.T) {
			data, err := os.ReadFile(kpPath)
			if err != nil {
				t.Fatal(err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]uint32{}
			n := len(f.Project.Maps)
			for _, i := range []int{0, n / 2, n - 1} {
				m := f.Project.Maps[i]
				p := fmt.Sprintf("project.maps[%d]", i)
				want[p+".cols"] = uint32(m.Cols)
				want[p+".rows"] = uint32(m.Rows)
				want[p+".start"] = m.Start
				want[p+".x.dataSource"] = uint32(m.X.DataSource)
				want[p+".y.addr"] = m.Y.Addr
				want[p+".x.signed"] = map[bool]uint32{true: 1}[m.X.Signed]
			}
			var paths []string
			for p := range want {
				paths = append(paths, p)
			}
			locs, err := f.Offsets(paths...)
			if err != nil {
				t.Fatal(err)
			}
			for p, w := range want {
				l := locs[p]
				buf := data
				if l.InMapBlock {
					buf = f.Project.intern
				}
				if l.InMapBlock != (f.Layout() == V2) {
					t.Errorf("%s: inMapBlock %v", p, l.InMapBlock)
				}
				got := binary.LittleEndian.Uint32(buf[l.Off:])
				if strings.HasSuffix(p, ".signed") {
					got &= 0xff
				}
				if got != w {
					t.Errorf("%s @0x%x: %d, want %d", p, l.Off, got, w)
				}
			}
			if _, err := f.Offset("project.maps[0].nope"); err == nil {
				t.Errorf("unknown path: no error")
			}
		})
	}
}

func diffAt(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}
