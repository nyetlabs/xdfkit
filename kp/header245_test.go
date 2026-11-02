package kp

import (
	"os"
	"testing"
)

func TestHeader245Parse(t *testing.T) {
	b, err := os.ReadFile("../testdata/local/8N0906018AN-0004-245.kp")
	if err != nil {
		t.Skip(err)
	}
	f, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if f.Layout() != V2 || f.HeaderID != header245 || f.HeaderLen != 0 {
		t.Fatalf("layout %v header %d HeaderLen %d", f.Layout(), f.HeaderID, f.HeaderLen)
	}
	if len(f.FixedHeader) != 0x5c-0x18 || f.Project.Name != "Original" {
		t.Fatalf("FixedHeader %d name %q", len(f.FixedHeader), f.Project.Name)
	}
	if n := len(f.Project.Maps); n != 4094 {
		t.Fatalf("maps %d", n)
	}
	m0 := f.Project.Maps[0]
	if m0.ID == "" || m0.Start == 0 || m0.LinkID != 0 || m0.X == nil || len(m0.X.MirrorPad) != 1 {
		t.Fatalf("maps[0] id=%q start=%#x link=%d pad=%v", m0.ID, m0.Start, m0.LinkID, m0.X)
	}
	var eeprom *Map
	for _, m := range f.Project.Maps {
		if m.ID == "ATISLATM" {
			eeprom = m
			break
		}
	}
	if eeprom == nil || eeprom.X == nil || eeprom.X.Precision != 2 {
		t.Fatalf("ATISLATM %#v", eeprom)
	}
	if n := len(f.Project.Folders); n != 316 {
		t.Fatalf("folders %d", n)
	}
	if f.Project.Folders[1].Name != "My maps" || len(f.Project.Folders[0].Pad) != 14 {
		t.Fatalf("folders[0].pad=%d folders[1]=%q", len(f.Project.Folders[0].Pad), f.Project.Folders[1].Name)
	}
	if _, err := f.Encode(); err == nil {
		t.Fatal("Encode of header 245 should fail")
	}
}
