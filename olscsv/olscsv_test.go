package olscsv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteSample(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "testdata", "olscsv", "sample.csv"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Read(b, "sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 2 {
		t.Fatalf("objects %d", len(m.Objects))
	}
	o0 := m.Objects[0]
	if o0.ID != "TSTA" || o0.Address != 0x1000 || o0.CPU == nil || *o0.CPU != 0x801000 || o0.VisLevel != 1 {
		t.Fatalf("obj0 %+v", o0)
	}
	o1 := m.Objects[1]
	if o1.ID != "TSTB" || o1.Cols != 4 || o1.Data.Bits != 16 || o1.CPU == nil || *o1.CPU != 0x802000 || o1.X == nil || o1.X.Source != "image" || o1.X.CPU == nil || *o1.X.CPU != 0x803000 {
		t.Fatalf("obj1 %+v x=%+v", o1, o1.X)
	}
	out, err := Write(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), Magic) {
		t.Fatalf("header %q", string(out[:min(40, len(out))]))
	}
	m2, err := Read(out, "round.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Objects) != 2 || m2.Objects[0].ID != "TSTA" || m2.Objects[0].CPU == nil || *m2.Objects[0].CPU != 0x801000 || m2.Objects[1].Address != 0x2000 {
		t.Fatalf("roundtrip %+v", m2.Objects)
	}
}

func TestDetectMagic(t *testing.T) {
	if !strings.HasPrefix(Magic, "Name;IdName;FolderName") {
		t.Fatal(Magic)
	}
}
