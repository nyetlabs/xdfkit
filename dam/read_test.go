package dam

import (
	"strings"
	"testing"
)

const sample = `
*** Created by ASAP2DAM Version 6.05 02.09.1998 ***
/EPR, {PROJ}, {1.0}, X
1, /REG, linear, {scale}, 0, 0, {raw}, 0
/REP, 1, 0, 0, 1, 0, 0;
2, /REG, recip, {time}, 0, 0, {s}, 0
/REP, 0, 2560, 10, 0, 0, 0;
3, /REG, odd, {}, 0, 0, {x}, 0
/REP, 1, 2, 3, 4, 0, 0;

1, /SPZ, VAL, {a value}, 1, $800010, $800010
/SPW, 1, -2, -1, 1
/SPX, 0, 0, 0, 0, 0

2, /SPZ, CUR, {a curve}, 2, $800100, $800100
/SPW, 2, 2, 0, 10
/SPX, 1, 1, 0, 4, 99
/FKX, 4, 0, 0

3, /SPZ, AX, {axis table}, 6, $800200, $800200
/SPW, 0, 0, 0, 0
/SPX, 2, -1, 0, 3, 99
/FKX, 5, 0, 0

4, /SPZ, BLK, {block}, 13, $800300, $800300
/SPW, 1, 1, 0, 9
/FKX, 16, 0, 0

5, /SPZ, CALIBID, {}, 12, $800400, $800400
/SPW, 0, 4, 0, 0

7, /SPZ, ODD, {odd}, 1, $800500, $800500
/SPW, 3, 1, 0, 1

6, /SPZ, MAP, {a map}, 3, $81C05C, $81C05C
/SPW, 1, 2, 0, 2
/SPX, 1, 1, 0, 10, 99
/SPY, 1, 2, 0, 20, 99
/FKX, 4, 0, 0
/FKY, 1, 0, 0

8, /SPZ, WMAP, {word counts}, 3, $800400, $800400
/SPW, 1, 2, 0, 1
/SPX, 1, 2, 0, 1, 0
/SPY, 1, 2, 0, 1, 0
/FKX, 4, 0, 0
/FKY, 2, 0, 0

9, /SPZ, SST, {shared}, 6, $800500, $800500
/SPW, 0, 0, 0, 0
/SPX, 2, 1, 0, 10, 0
/FKX, 8, 0, 0

10, /SPZ, ONSST, {on the shared axis}, 7, $800600, $800600
/SPW, 1, 1, 0, 1
/SPX, 2, 1, 0, 10, 9

11, /SPZ, SPLIT, {two addresses}, 1, $800700, $800800
/SPW, 1, 1, 0, 1
`

func TestRead(t *testing.T) {
	m, warnings, err := Read([]byte(sample), "demo.dam")
	if err != nil {
		t.Fatal(err)
	}
	if m.Provenance.Format != "damos" || m.Provenance.Origin != "damos" || m.Project.Name != "PROJ" || m.Project.Version != "1.0" {
		t.Fatalf("header %+v %+v", m.Provenance, m.Project)
	}
	by := map[string]int{}
	for i, o := range m.Objects {
		by[o.ID] = i
	}
	val := m.Objects[by["VAL"]]
	if val.Shape != "value" || val.Address != 0x10 || val.Data.Bits != 16 || !val.Data.Signed || val.Data.Endian != "little" || val.Value.Conversion.Factor != 1 {
		t.Fatalf("value %+v %+v", val, val.Value.Conversion)
	}
	cur := m.Objects[by["CUR"]]
	// count, 4 axis bytes, pad, 16-bit values
	if cur.Shape != "1d" || cur.Cols != 4 || cur.Address != 0x106 || cur.X == nil || cur.X.Address == nil || *cur.X.Address != 0x101 {
		t.Fatalf("curve addr %x axis %+v", cur.Address, cur.X)
	}
	if !cur.Value.Conversion.Reciprocal || cur.Value.Conversion.Factor != 256 || cur.Value.Units != "s" {
		t.Fatalf("curve conversion %+v", cur.Value)
	}
	ax := m.Objects[by["AX"]]
	if ax.Shape != "1d" || ax.Cols != 5 || ax.Address != 0x201 || ax.Data.Bits != 8 || !ax.Data.Signed {
		t.Fatalf("axis table %+v", ax)
	}
	blk := m.Objects[by["BLK"]]
	if blk.Shape != "1d" || blk.Cols != 16 || blk.Address != 0x300 || blk.X == nil || blk.X.Source != "ordinal" {
		t.Fatalf("block %+v", blk)
	}
	mp := m.Objects[by["MAP"]]
	if mp.Shape != "2d" || mp.Cols != 1 || mp.Rows != 4 || mp.Address != 0x1C064 {
		t.Fatalf("map %+v", mp)
	}
	if mp.Y == nil || mp.Y.Address == nil || *mp.Y.Address != 0x1C05E || mp.X == nil || mp.X.Address == nil || *mp.X.Address != 0x1C062 {
		t.Fatalf("map axes x %+v y %+v", mp.X, mp.Y)
	}
	if _, ok := by["CALIBID"]; ok {
		t.Fatal("type 12 was kept")
	}
	wm := m.Objects[by["WMAP"]]
	if wm.Address != 0x410 || wm.Rows != 4 || wm.Cols != 2 || wm.Y == nil || *wm.Y.Address != 0x404 || *wm.X.Address != 0x40C {
		t.Fatalf("word map addr %x x %v y %v", wm.Address, wm.X, wm.Y)
	}
	split := m.Objects[by["SPLIT"]]
	if split.Address != 0x800 {
		t.Fatalf("split address %x", split.Address)
	}
	sh := m.Objects[by["ONSST"]]
	sst := m.Objects[by["SST"]]
	if sh.Shape != "1d" || sh.Cols != 8 || sh.Address != 0x600 || sh.X == nil || sh.X.Address == nil || *sh.X.Address != sst.Address || sst.Address != 0x501 {
		t.Fatalf("shared %+v sst %x", sh, sst.Address)
	}
	order, odd, skip := false, false, false
	for _, w := range warnings {
		if strings.Contains(w, "byte order") {
			order = true
		}
		if strings.Contains(w, "not linear") {
			odd = true
		}
		if strings.Contains(w, "type 12") {
			skip = true
		}
	}
	if !order || !odd || !skip {
		t.Fatalf("warnings %v", warnings)
	}
}

func TestNotDAMOS(t *testing.T) {
	if _, _, err := Read([]byte("hello\n"), "x"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCP850Descriptions(t *testing.T) {
	// 0x9A Ü, 0x94 ö, 0xE1 ß (CP850); 0xB0 ° (Latin-1, not CP850 shade).
	m, _, err := Read([]byte("*** ASAP2DAM ***\n/EPR, {P}, {1}, X\n"+
		"1, /REG, lin, {}, 0, 0, {}, 0\n/REP, 1, 0, 0, 1, 0, 0;\n"+
		"1, /SPZ, KOE, {\x9abertragung \xb0KW Gr\x94\xe1e}, 1, $800010, $800010\n"+
		"/SPW, 1, 1, 0, 1\n/SPX, 0, 0, 0, 0, 0\n"), "cp850.dam")
	if err != nil {
		t.Fatal(err)
	}
	want := "Übertragung °KW Größe"
	if m.Objects[0].Description != want {
		t.Fatalf("description %q want %q", m.Objects[0].Description, want)
	}
}
