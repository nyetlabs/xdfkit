package kp

import (
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
)

func TestFixtures(t *testing.T) {
	root := testenv.ModuleRoot()
	dir := filepath.Join(root, "testdata", "kp")
	for _, c := range []struct {
		file   string
		header int32
		maps   int
	}{
		{"v1-fewmaps.kp", header116, 4},
		{"v2-fewmaps.kp", header292, 4},
		{"245-fewmaps.kp", header245, 4},
		{"503-fewmaps.kp", header503, 4},
		{"597-fewmaps.kp", header597, 4},
	} {
		b, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		f, err := Parse(b)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if f.HeaderID != c.header || len(f.Project.Maps) != c.maps {
			t.Fatalf("%s header %d maps %d, want %d / %d",
				c.file, f.HeaderID, len(f.Project.Maps), c.header, c.maps)
		}
		if len(f.Project.Folders) < 1 {
			t.Fatalf("%s: no folders", c.file)
		}
	}
}
