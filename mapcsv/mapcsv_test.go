package mapcsv

import (
	"bytes"
	"encoding/csv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/model"
)

// Columns the archived mapdump CSVs got wrong: axis addresses, units and
// scales of mirrored axes and unused slots (docs/kp-format.md). Also skipped:
// the raw range of 32-bit cells, and the value range of 4Z7907551R, whose CSV
// was made with a patched image. Objects without cells expect "-".
var axisCols = map[int]bool{7: true, 8: true, 9: true, 10: true, 12: true, 13: true}

var refs = map[string][]string{
	"8D0907551H": {"4Z7907551R", "8D0907551F", "8D0907551G"},
	"8D0907551M": {"4Z7907551R", "8D0907551F", "8D0907551G"},
}

func readModel(t *testing.T, dir, stem string) *model.Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, stem+".kp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.ReadKP(b, stem+".kp")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func parse(t *testing.T, b []byte) [][]string {
	t.Helper()
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// TestArchive compares the CSV of each archived pack with mapdump's.
func TestArchive(t *testing.T) {
	dir := testenv.Archive(t)
	csvs, _ := filepath.Glob(filepath.Join(dir, "*.csv"))
	for _, path := range csvs {
		stem := strings.TrimSuffix(filepath.Base(path), ".csv")
		if strings.Contains(stem, "-") {
			continue // dated revisions: no image entry
		}
		t.Run(stem, func(t *testing.T) {
			img := corpus.ArchiveImage(t, stem)
			var rs []Ref
			for _, r := range refs[stem] {
				rs = append(rs, Ref{r + ".kp", readModel(t, dir, r)})
			}
			got, err := Write(readModel(t, dir, stem), img, rs)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			g, w := parse(t, got), parse(t, want)
			if len(g) != len(w) {
				t.Fatalf("%d rows, want %d", len(g), len(w))
			}
			bad := 0
			for i := range w {
				if strings.HasPrefix(w[i][3], "0x") || strings.HasSuffix(w[i][3], "x0") {
					w[i][14], w[i][15], w[i][16], w[i][17] = "-", "-", "-", "-"
				}
				for j := range max(len(g[i]), len(w[i])) {
					if axisCols[j] || (j == 16 || j == 17) && strings.HasPrefix(w[i][4], "32 Bit") ||
						j >= 14 && j <= 17 && stem == "4Z7907551R" {
						continue
					}
					if j >= len(g[i]) || j >= len(w[i]) || g[i][j] != w[i][j] {
						if bad++; bad <= 5 {
							t.Errorf("row %d col %d: got %q, want %q", i, j, at(g[i], j), at(w[i], j))
						}
					}
				}
			}
		})
	}
}

func at(r []string, i int) string {
	if i < len(r) {
		return r[i]
	}
	return "<missing>"
}

func TestJavaDouble(t *testing.T) {
	for f, want := range map[float64]string{
		0: "0.0", 1: "1.0", -2.5: "-2.5", 0.0078125: "0.0078125", 206.70000000000002: "206.70000000000002",
		2331244787: "2.331244787E9", 1e7: "1.0E7", 9999999: "9999999.0", 1e-3: "0.001", 1e-4: "1.0E-4",
		1.5e-10: "1.5E-10", math.Inf(1): "Infinity", math.Copysign(0, -1): "-0.0",
	} {
		if got := javaDouble(f); got != want {
			t.Errorf("javaDouble(%v) = %q, want %q", f, got, want)
		}
	}
}
