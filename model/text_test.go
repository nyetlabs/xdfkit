package model

import (
	"testing"

	"go.nyet.org/xdfkit/kp"
)

func cleanFileDisplay(f *kp.File) {
	p := &f.Project
	p.Name = CleanText(p.Name)
	p.Version = CleanText(p.Version)
	for _, fd := range p.Folders {
		fd.Name = CleanText(fd.Name)
	}
	for _, m := range p.Maps {
		c := mapDisplay(m)
		*m = *c
	}
}

func TestCleanText(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"KFZW 16", "KFZW 16"},
		{"  Init-Wert  Start  ", "Init-Wert Start"},
		{"CARB:\tTank", "CARB: Tank"},
		{"a\n  b \t c\n", "a\nb c\n"},
		{"a\r\n  b\t\tb", "a\nb b"},
		{"Byte0  EV6    EV3", "Byte0 EV6 EV3"},
	} {
		if got := CleanText(c.in); got != c.want {
			t.Errorf("CleanText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	m := &Model{
		Project:    Project{Name: "  P  Q  ", Version: "1  "},
		Categories: []*Category{{Name: "ARMD: 10.50     Antiruckel"}},
		Objects: []*Object{{
			Description: "gut,  AHKAT",
			Comment:     "Byte0  EV6    EV3\r\nByte1  XX",
			Value:       Value{Units: "  U/min  "},
			X:           &Axis{Value: Value{Description: "  rpm  "}},
		}},
	}
	m.CleanDisplay()
	o := m.Objects[0]
	if m.Project.Name != "P Q" || m.Project.Version != "1" || m.Categories[0].Name != "ARMD: 10.50 Antiruckel" {
		t.Fatalf("project/category %q %q %q", m.Project.Name, m.Project.Version, m.Categories[0].Name)
	}
	if o.Description != "gut, AHKAT" || o.Value.Units != "U/min" || o.X.Value.Description != "rpm" {
		t.Fatalf("object %q %q %q", o.Description, o.Value.Units, o.X.Value.Description)
	}
	if o.Comment != "Byte0  EV6    EV3\r\nByte1  XX" {
		t.Fatalf("comment %q", o.Comment)
	}
}
