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
// (keyed by map ID and axis, as the prototype did). MP01, which the prototype
// lacked, counts map IDs with a stale end address. RC01, which it lacked too,
// counts maps and axes at odd addresses, and takes over the prototype's DS03
// and XR01 on KFFRMIN X and LAMFA X (8D0907551G, 4Z7907551R), XR01 on LAMFA X
// (06A906032HS, 8N0906018CB) and on KFMLDMN Y (4D1907558). RC02 finds
// nothing in these packs. 8D0907558M's DS02 and AX01 differ: the prototype read
// "subtract" axes as forward sums, and six axes that are increasing that way
// but negative as WinOLS reads them are AX01 here. AX01 counts a previous
// byte only when it is a plausible count or the axis declares a header.
var want = map[string]map[string]int{
	"06A906032HS": {"DS03": 7, "RC01": 26},
	"06A906032LP": {"DS01": 16, "DS03": 6, "XR01": 1, "RC01": 30},
	"4D1907558":   {"DS03": 1, "AX02": 3, "XR01": 5, "RC01": 6},
	"4Z7907551AA": {"XR01": 1, "MP01": 26, "RC01": 4},
	"4Z7907551R":  {"DS01": 24, "RC01": 44},
	"8D0907551F":  {"DS01": 20, "XR01": 2, "RC01": 45},
	"8D0907551G":  {"DS01": 23, "RC01": 18},
	"8D0907551H":  {"XR01": 3},
	"8D0907551K":  {"XR01": 2, "MP01": 64},
	"8D0907551M":  {"DS01": 1, "XR01": 3, "MP01": 13},
	"8D0907558E":  {"DS02": 216},
	"8D0907558M":  {"DS02": 68, "AX01": 10, "XR01": 9, "MP01": 333},
	"8N0906018CB": {"DS01": 13, "RC01": 25},
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

func TestAxisRefKey(t *testing.T) {
	for _, c := range fixtureCases(t) {
		if c.Name != "me7" {
			continue
		}
		f, img := c.build(t)
		r, err := Lint(f, img, Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, fd := range r.Findings {
			for _, ref := range fd.Maps {
				if ref.Key == "" {
					t.Errorf("%s: empty key for %s", fd.ID, ref.ID)
				}
				if ref.Key != ref.ID {
					t.Errorf("%s: key %q, want id %q", fd.ID, ref.Key, ref.ID)
				}
			}
		}
	}
}

func TestCountDisagrees(t *testing.T) {
	n := 8
	bare := &kp.Axis{}
	for _, b := range []int{0, 32, 185, 255} {
		if countDisagrees(bare, &b, n) {
			t.Errorf("byte %d flagged as a count", b)
		}
	}
	bad := 9
	if !countDisagrees(bare, &bad, n) {
		t.Error("a plausible count that disagrees was kept")
	}
	if countDisagrees(bare, &n, n) {
		t.Error("a matching count was flagged")
	}
	declared := &kp.Axis{DataHeader: 1}
	odd := 255
	if !countDisagrees(declared, &odd, n) {
		t.Error("a declared header that disagrees was kept")
	}
}

func jsonOf(t *testing.T, f *kp.File) string {
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
