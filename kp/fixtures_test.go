package kp

import (
	"os"
	"path/filepath"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
)

func TestOLS5SizesFor(t *testing.T) {
	for _, c := range []struct {
		id                int32
		name, ver, marked int
	}{
		{503, 84, 84, 65},
		{597, 93, 93, 69},
		{839, 105, 109, 0},
		{504, 84, 84, 65}, // other 50x follows 503
	} {
		s := ols5SizesFor(c.id)
		if s.NameTrailer != c.name || s.VersionTrailer != c.ver || s.MarkedExtra != c.marked {
			t.Fatalf("id %d: %+v, want name=%d ver=%d marked=%d", c.id, s, c.name, c.ver, c.marked)
		}
	}
}

func TestFixtures(t *testing.T) {
	root := testenv.ModuleRoot()
	dir := filepath.Join(root, "testdata", "kp")
	for _, c := range []struct {
		file   string
		header int32
		maps   int
	}{
		{"v1-fewmaps.kp", 116, 4},
		{"v2-fewmaps.kp", 292, 4},
		{"245-fewmaps.kp", 245, 4},
		{"503-fewmaps.kp", 503, 4},
		{"597-fewmaps.kp", 597, 4},
	} {
		b, err := os.ReadFile(filepath.Join(dir, c.file))
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		f, err := Parse(b)
		if err != nil {
			t.Fatalf("%s: %v", c.file, err)
		}
		if !knownHeader(f.HeaderID) {
			t.Fatalf("%s: header %d not in knownHeaders", c.file, f.HeaderID)
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
