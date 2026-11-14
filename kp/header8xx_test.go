package kp

import (
	"os"
	"path/filepath"
	"testing"
)

func parseLocalKP(t *testing.T, parts ...string) *File {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "testdata", "local"}, parts...)...))
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHeader839Parse(t *testing.T) {
	f := parseLocalKP(t, "incoming", "csv", "06A906032HJ 0002.kp")
	if f.Layout() != OLS5 || f.HeaderID != 839 {
		t.Fatalf("layout %v header %d", f.Layout(), f.HeaderID)
	}
	if len(f.Project.Maps) != 4786 {
		t.Fatalf("maps %d", len(f.Project.Maps))
	}
	m := f.Project.Maps[0]
	if m.ID != "A0" || m.Start != 0x1609c || m.RWin != 2 || m.ViewMode != 1 || !m.Signed {
		t.Fatalf("map0 id=%q start=%x rwin=%d vm=%d signed=%v", m.ID, m.Start, m.RWin, m.ViewMode, m.Signed)
	}
	if len(f.Project.NameTrailer) != 105 || len(f.Project.VersionTrailer) != 109 {
		t.Fatalf("trailers %d %d", len(f.Project.NameTrailer), len(f.Project.VersionTrailer))
	}
	if m.ols5().PreX != 0 {
		t.Fatalf("839 PreX=%#x", m.ols5().PreX)
	}
	if _, err := f.Encode(); err == nil {
		t.Fatal("Encode of 8xx should fail")
	}
}

func TestHeader855Parse(t *testing.T) {
	f := parseLocalKP(t, "incoming", "csvgaps", "new good export all", "all maps KFKHFM in difference view.kp")
	if f.HeaderID != 855 || f.Layout() != OLS5 {
		t.Fatalf("layout %v header %d", f.Layout(), f.HeaderID)
	}
	if len(f.Project.Maps) != 4947 {
		t.Fatalf("maps %d", len(f.Project.Maps))
	}
	if f.Project.Maps[0].ols5().PreX != preXMagic {
		t.Fatalf("map0 PreX=%#x", f.Project.Maps[0].ols5().PreX)
	}
	var diff, pct int
	for _, m := range f.Project.Maps {
		if m.Difference {
			diff++
		}
		if m.Percent {
			pct++
		}
	}
	if diff != 2 || pct != 1 {
		t.Fatalf("difference=%d percent=%d", diff, pct)
	}
	if _, err := f.Encode(); err == nil {
		t.Fatal("Encode of 8xx should fail")
	}
}

func TestHeader855OriginalIgnore(t *testing.T) {
	kfkhfm := func(name string) *Map {
		f := parseLocalKP(t, "incoming", "csvgaps", "new good export all", name)
		for _, m := range f.Project.Maps {
			if m.ID == "KFKHFM" {
				return m
			}
		}
		t.Fatal("KFKHFM missing")
		return nil
	}
	n, ig := kfkhfm("all maps KFKHFM in normal view.kp"), kfkhfm("all maps KFKHFM in ignore factor offset view.kp")
	if n.Original != 0 || ig.Original != 1 || n.Difference || ig.Difference || n.Percent || ig.Percent {
		t.Fatalf("Original normal=%d ignore=%d diff/pct=%v/%v %v/%v",
			n.Original, ig.Original, n.Difference, ig.Difference, n.Percent, ig.Percent)
	}
}
