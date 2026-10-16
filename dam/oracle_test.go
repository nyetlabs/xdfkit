package dam

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/model"
	"go.nyet.org/xdfkit/xdf"
)

// The .dam files are committed under testdata/damos. The oracle is the
// corpus definition (JSON) and, when the archive has one, the KP. The same
// model is written to KP and XDF and read back.
func TestOracle(t *testing.T) {
	root := testenv.ModuleRoot()
	cases := []struct {
		file, def, kp string
	}{
		{"06A906032S-0004.dam", "06A906032S-0004.json", ""},
		{"06A906032HF-0006.dam", "06A906032HF-0006.json", ""},
		{"8D0907551M-0002.dam", "8D0907551M-0002.json", "8D0907551M.kp"},
	}
	var defs *corpus.Corpus
	for _, c := range cases {
		b, err := os.ReadFile(filepath.Join(root, "testdata", "damos", c.file))
		if err != nil || len(b) == 0 {
			testenv.Missing(t, "testdata/damos/%s", c.file)
		}
		got, _, err := Read(b, c.file)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if defs == nil {
			defs = corpus.Open(t)
		}
		refb, err := os.ReadFile(filepath.Join(defs.Dir, "defs", c.def))
		if err != nil {
			t.Errorf("%s: %v", c.def, err)
			continue
		}
		ref := new(model.Model)
		if err := json.Unmarshal(refb, ref); err != nil {
			t.Errorf("%s: %v", c.def, err)
			continue
		}
		if ref.Provenance == nil || (ref.Provenance.Origin != "damos" && ref.Provenance.Origin != "manual") {
			t.Errorf("%s: origin %v", c.def, ref.Provenance)
		}
		// 06A906032S is the DAMOS dataset the corpus definition was converted from.
		// 06A906032HF-0006.dam is an older 4019.XX export, so its definition is only reported.
		report(t, filepath.Base(c.file)+" json", got, ref, c.def == "06A906032S-0004.json")
		if c.kp != "" {
			kb, err := os.ReadFile(filepath.Join(root, "testdata/archive/ecuxplot", c.kp))
			if err != nil {
				t.Errorf("%s: %v", c.kp, err)
			} else if kp, err := model.ReadKP(kb, c.kp); err != nil {
				t.Errorf("%s: %v", c.kp, err)
			} else {
				report(t, c.kp, got, kp, false)
			}
		}
		roundTrip(t, c.file, got)
	}
}

func roundTrip(t *testing.T, name string, want *model.Model) {
	t.Helper()
	f, err := want.KP(nil)
	if err != nil {
		t.Errorf("%s kp: %v", name, err)
		return
	}
	kb, err := f.Encode()
	if err != nil {
		t.Errorf("%s kp encode: %v", name, err)
		return
	}
	back, err := model.ReadKP(kb, name)
	if err != nil {
		t.Errorf("%s kp read: %v", name, err)
	} else {
		report(t, name+" kp", back, want, true)
	}
	xb, err := xdf.Write(want, nil, name)
	if err != nil {
		t.Errorf("%s xdf: %v", name, err)
		return
	}
	meta, err := xdf.Meta(want, xb, "xdfkit test")
	if err != nil {
		t.Errorf("%s meta: %v", name, err)
		return
	}
	back, _, err = xdf.Read(xb, meta)
	if err != nil {
		t.Errorf("%s xdf read: %v", name, err)
		return
	}
	report(t, name+" xdf", back, want, true)
}

func report(t *testing.T, label string, got, ref *model.Model, strict bool) {
	t.Helper()
	at := map[string]*model.Object{}
	for _, o := range got.Objects {
		if at[o.ID] == nil {
			at[o.ID] = o
		}
	}
	var addr, store, conv, axis, shape, shared, missing, n int
	var sample []string
	for _, o := range ref.Objects {
		g := at[o.ID]
		if g == nil {
			missing++
			continue
		}
		n++
		if g.Address != o.Address {
			addr++
			if len(sample) < 8 {
				sample = append(sample, o.ID+" address")
			}
		}
		if g.Data.Bits != o.Data.Bits || g.Data.Signed != o.Data.Signed {
			store++
			if len(sample) < 8 {
				sample = append(sample, o.ID+" storage")
			}
		}
		if !sameConv(g.Value.Conversion, o.Value.Conversion) {
			conv++
			if len(sample) < 8 {
				sample = append(sample, o.ID+" conversion")
			}
		}
		if o.X != nil && o.X.Address != nil && (g.X == nil || g.X.Address == nil || *g.X.Address != *o.X.Address) {
			axis++
		}
		if o.Y != nil && o.Y.Address != nil && (g.Y == nil || g.Y.Address == nil || *g.Y.Address != *o.Y.Address) {
			axis++
		}
		if g.Shape != o.Shape || g.Rows != o.Rows || g.Cols != o.Cols {
			if g.X != nil && g.X.Source == "image" && o.X == nil {
				shared++
			} else {
				shape++
				if len(sample) < 8 {
					sample = append(sample, o.ID+" shape")
				}
			}
		}
	}
	t.Logf("%s: %d names, missing %d, address %d, storage %d, conversion %d, axis %d, shape %d, shared-axis filled %d",
		label, n, missing, addr, store, conv, axis, shape, shared)
	if !strict {
		return
	}
	if addr > 0 || axis > 0 || shape > 0 {
		t.Errorf("%s: address %d, axis %d, shape %d (%s)", label, addr, axis, shape, strings.Join(sample, ", "))
	}
}

func sameConv(a, b model.Conversion) bool {
	if a.Reciprocal != b.Reciprocal {
		return false
	}
	return near(a.Factor, b.Factor) && near(a.Offset, b.Offset)
}

func near(a, b float64) bool {
	d := math.Abs(a - b)
	s := math.Abs(a) + math.Abs(b)
	if s < 1e-9 {
		return d < 1e-9
	}
	return d/s < 1e-6
}
