package dam

import (
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
)

func TestFragments(t *testing.T) {
	root := testenv.ModuleRoot()
	for _, c := range []struct {
		file string
		min  int
	}{
		{"basic.dam", 3},
		{"shared-axis.dam", 2},
		{"cp850.dam", 1},
	} {
		p := filepath.Join(root, "testdata", "damos", c.file)
		b, err := os.ReadFile(p)
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v", c.file, err)
		}
		m, _, err := Read(b, c.file)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if len(m.Objects) < c.min {
			t.Errorf("%s: %d objects, want >= %d", c.file, len(m.Objects), c.min)
		}
		switch c.file {
		case "cp850.dam":
			want := "Übertragung °KW Größe"
			if m.Objects[0].Description != want {
				t.Errorf("cp850 description %q want %q", m.Objects[0].Description, want)
			}
		case "shared-axis.dam":
			by := map[string]int{}
			for i, o := range m.Objects {
				by[o.ID] = i
			}
			sh, sst := m.Objects[by["ONSST"]], m.Objects[by["SST"]]
			if sh.X == nil || sh.X.Address == nil || *sh.X.Address != sst.Address {
				t.Errorf("shared axis %+v sst %x", sh.X, sst.Address)
			}
		}
		t.Logf("%s: %d objects", c.file, len(m.Objects))
	}
}
