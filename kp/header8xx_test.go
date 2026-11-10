package kp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHeader839Parse(t *testing.T) {
	path := filepath.Join("..", "testdata", "local", "incoming", "csv", "06A906032HJ 0002.kp")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Layout() != OLS5 || !is8xx(f.HeaderID) || f.HeaderID != 839 {
		t.Fatalf("layout %v header %d", f.Layout(), f.HeaderID)
	}
	if len(f.Project.Maps) != 4786 {
		t.Fatalf("maps %d", len(f.Project.Maps))
	}
	m := f.Project.Maps[0]
	if m.ID != "A0" || m.Start != 0x1609c {
		t.Fatalf("map0 id=%q start=%x", m.ID, m.Start)
	}
	if m.RWin != 2 || m.ViewMode != 1 || !m.Signed {
		t.Fatalf("map0 rwin=%d vm=%d signed=%v", m.RWin, m.ViewMode, m.Signed)
	}
	if len(f.Project.NameTrailer) != 105 || len(f.Project.VersionTrailer) != 109 {
		t.Fatalf("trailers %d %d", len(f.Project.NameTrailer), len(f.Project.VersionTrailer))
	}
	if _, err := f.Encode(); err == nil {
		t.Fatal("Encode of 8xx should fail")
	}
}
