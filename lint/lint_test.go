package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/kp"
)

// want counts map axes per rule, from the prototype run on ecuxplot's data
// (the archived packs, with OEM images from the corpus)
// (keyed by map ID and axis, as the prototype did). R8, which the prototype
// lacked, counts map IDs with a stale end address. 8D0907558M's R2 and R4
// differ: the prototype read "subtract" axes as forward sums, and six axes
// that are increasing that way but negative as WinOLS reads them are R4 here.
var want = map[string]map[string]int{
	"06A906032HS": {"R3": 7, "R6": 1},
	"06A906032LP": {"R1": 16, "R3": 6, "R6": 1},
	"4D1907558":   {"R3": 1, "R6": 9},
	"4Z7907551AA": {"R6": 1, "R8": 26},
	"4Z7907551R":  {"R1": 24, "R3": 1, "R6": 1},
	"8D0907551F":  {"R1": 20, "R6": 2},
	"8D0907551G":  {"R1": 23, "R3": 1, "R6": 1},
	"8D0907551H":  {"R6": 3},
	"8D0907551K":  {"R6": 2, "R8": 64},
	"8D0907551M":  {"R1": 1, "R6": 3, "R8": 13},
	"8D0907558E":  {"R2": 216},
	"8D0907558M":  {"R2": 68, "R4": 45, "R6": 3, "R8": 333},
	"8N0906018CB": {"R1": 13, "R6": 1},
}

func load(t *testing.T, stem string) (*kp.File, []byte) {
	t.Helper()
	d := testenv.Archive(t)
	b, err := os.ReadFile(filepath.Join(d, stem+".kp"))
	if err != nil {
		testenv.Missing(t, "%v", err)
	}
	img := corpus.ArchiveImage(t, stem)
	f, err := kp.Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return f, img
}

func TestLint(t *testing.T) {
	for stem, w := range want {
		t.Run(stem, func(t *testing.T) {
			f, img := load(t, stem)
			r, err := Lint(f, img, Options{})
			if err != nil {
				t.Fatal(err)
			}
			wantFam := ME7
			if strings.HasPrefix(stem, "8D0907558") {
				wantFam = M3
			}
			if r.Family != wantFam {
				t.Errorf("family %s (%s), want %s", r.Family, r.FamilySource, wantFam)
			}
			got := map[string]int{}
			seen := map[string]bool{}
			for _, fd := range r.Findings {
				for _, m := range fd.Maps {
					k := m.ID
					if k == "" {
						k = `"` + m.Name + `"`
					}
					k += "/" + m.Axis
					if !seen[k] {
						seen[k] = true
						got[fd.Rule]++
					}
				}
			}
			if !reflect.DeepEqual(got, w) {
				t.Errorf("got %v, want %v", got, w)
			}
		})
	}
}

// TestApply fixes the high-confidence findings, re-encodes, and checks that
// the output parses to the fixed file and that those findings are gone.
func TestApply(t *testing.T) {
	for _, stem := range []string{"8D0907551G", "8D0907551M", "8D0907558E"} {
		t.Run(stem, func(t *testing.T) {
			f, img := load(t, stem)
			r, err := Lint(f, img, Options{})
			if err != nil {
				t.Fatal(err)
			}
			sel := Select(r.Findings, nil, nil, High)
			if len(sel) == 0 {
				t.Fatal("nothing to fix")
			}
			if err := Apply(f, sel); err != nil {
				t.Fatal(err)
			}
			out, err := f.Encode()
			if err != nil {
				t.Fatal(err)
			}
			g, err := kp.Parse(out)
			if err != nil {
				t.Fatal(err)
			}
			if jsonOf(t, f) != jsonOf(t, g) {
				t.Error("re-parsed output differs from the fixed file")
			}
			r2, err := Lint(g, img, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if left := Select(r2.Findings, nil, nil, High); len(left) != 0 {
				t.Errorf("%d high-confidence findings left, first %s", len(left), left[0].ID)
			}
			if err := Apply(g, sel); err != nil {
				t.Errorf("reapplying: %v", err)
			}
		})
	}
}

func jsonOf(t *testing.T, f *kp.File) string {
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
