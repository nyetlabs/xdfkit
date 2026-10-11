package model

import "testing"

func obj(key string, addr Addr, factor float64) *Object {
	return &Object{
		Key: key, ID: key, Shape: "value", Address: addr, Rows: 1, Cols: 1,
		Data:  Data{Bits: 8},
		Value: Value{Units: "rpm", Conversion: Conversion{Factor: factor}},
	}
}

func acts(t *testing.T, as []Action, want string) {
	t.Helper()
	for _, a := range as {
		if a.Action == want || a.Field == want {
			return
		}
	}
	t.Errorf("no action %s in %v", want, as)
}

func TestMergeAddAndMissing(t *testing.T) {
	base := &Model{Schema: SchemaID, Objects: []*Object{obj("KFZW", 0x100, 0.75)}}
	in := &Model{Schema: SchemaID, Objects: []*Object{obj("LDRXN", 0x200, 1)}}
	out, as, err := Merge(base, in, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 2 || out.Objects[1].Key != "LDRXN" {
		t.Fatalf("objects %v", out.Objects)
	}
	acts(t, as, "added")
	acts(t, as, "missing")
	acts(t, as, "conversion")
}

func TestMergeDescriptionKeepsAddress(t *testing.T) {
	base := &Model{Schema: SchemaID, Provenance: &Provenance{Format: "kp", Origin: "hand", SHA256: "aa"},
		Objects: []*Object{obj("KFZW", 0x100, 0.75)}}
	base.Objects[0].Description = "old"
	in := obj("KFZW", 0x999, 1)
	in.Description = "new sentence"
	inc := &Model{Schema: SchemaID, Objects: []*Object{in}}
	out, as, err := Merge(base, inc, MergeOptions{Description: "replace", Comment: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 {
		t.Fatalf("len %d", len(out.Objects))
	}
	o := out.Objects[0]
	if o.Address != 0x100 || o.Value.Conversion.Factor != 0.75 || o.Description != "new sentence" {
		t.Fatalf("%#v", o)
	}
	if out.Provenance.Origin != "hand" {
		t.Fatal(out.Provenance)
	}
	acts(t, as, "description")
}

func TestMergeUpdateMovesAddress(t *testing.T) {
	base := &Model{Schema: SchemaID, Objects: []*Object{obj("KFZW", 0x100, 0.75)}}
	base.Objects[0].View.Base = 16
	base.Objects[0].Value.Units = "grad KW"
	in := obj("KFZW", 0x200, 1)
	in.Data.Signed = true
	in.Rows, in.Cols = 1, 4
	in.Value.Units = ""
	inc := &Model{Schema: SchemaID, Objects: []*Object{in}}
	out, as, err := Merge(base, inc, MergeOptions{Update: true})
	if err != nil {
		t.Fatal(err)
	}
	o := out.Objects[0]
	if o.Address != 0x200 || !o.Data.Signed || o.Cols != 4 {
		t.Fatalf("layout %#v", o)
	}
	if o.Value.Conversion.Factor != 0.75 || o.View.Base != 16 || o.Value.Units != "grad KW" {
		t.Fatalf("kept display %#v", o.Value)
	}
	acts(t, as, "address")
	acts(t, as, "units")
}

func TestMergeAmbiguous(t *testing.T) {
	a, b := obj("KFZW", 0x100, 0.75), obj("KFZW", 0x180, 0.75)
	base := &Model{Schema: SchemaID, Objects: []*Object{a, b}}
	in := obj("KFZW", 0x300, 1)
	out, as, err := Merge(base, &Model{Schema: SchemaID, Objects: []*Object{in}}, MergeOptions{Update: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Objects[0].Address != 0x100 || out.Objects[1].Address != 0x180 || len(out.Objects) != 2 {
		t.Fatalf("%v", out.Objects)
	}
	acts(t, as, "ambiguous")
}

func TestMergeSmart(t *testing.T) {
	fuel := &Category{ID: 0, Name: "Fuel"}
	boost := &Category{ID: 1, Name: "Boost"}
	a := obj("KFZW", 0x100, 0.75)
	a.Categories = []int{0}
	b := obj("KFZW", 0x100, 0.75)
	b.Categories = []int{1}
	m := &Model{Schema: SchemaID, Categories: []*Category{fuel, boost}, Objects: []*Object{a, b}}
	out, as, err := Merge(m, nil, MergeOptions{Smart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 || len(out.Objects[0].Categories) != 2 {
		t.Fatalf("%#v", out.Objects[0])
	}
	acts(t, as, "categories")

	deg := obj("LDR", 0x400, 0.75)
	deg.Value.Units = "deg"
	afr := obj("LDR", 0x400, 2)
	afr.Value.Units = "afr"
	m = &Model{Schema: SchemaID, Objects: []*Object{deg, afr}}
	out, as, err = Merge(m, nil, MergeOptions{Smart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 || len(out.Objects[0].Alternates) != 1 || out.Objects[0].Alternates[0].Value.Units != "afr" {
		t.Fatalf("%#v", out.Objects[0])
	}
	acts(t, as, "alternate")

	signed := obj("X", 0x500, 1)
	signed.Data.Signed = true
	plain := obj("X", 0x500, 1)
	m = &Model{Schema: SchemaID, Objects: []*Object{plain, signed}}
	out, _, err = Merge(m, nil, MergeOptions{Smart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 2 {
		t.Fatalf("signed kept %d", len(out.Objects))
	}
}

func TestMergeOverlapAndSpan(t *testing.T) {
	base := &Model{Schema: SchemaID, Objects: []*Object{obj("A", 0x100, 1)}}
	base.Objects[0].Cols = 4
	in := obj("B", 0x101, 1)
	out, as, err := Merge(base, &Model{Schema: SchemaID, Objects: []*Object{in}}, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 2 {
		t.Fatal(len(out.Objects))
	}
	acts(t, as, "overlap")

	dup := obj("C", 0x100, 1)
	dup.Cols = 4
	out, as, err = Merge(base, &Model{Schema: SchemaID, Objects: []*Object{dup}}, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 {
		t.Fatalf("dup %d", len(out.Objects))
	}
	acts(t, as, "skipped")
}

func TestMergeBlankAndDash(t *testing.T) {
	blank := func(addr Addr) *Object {
		o := obj("x", addr, 1)
		o.Key, o.ID = "", ""
		return o
	}
	base := &Model{Schema: SchemaID, Objects: []*Object{blank(0x100)}}
	in := blank(0x200)
	in.Description = "other"
	out, _, err := Merge(base, &Model{Schema: SchemaID, Objects: []*Object{in}}, MergeOptions{Description: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 2 || out.Objects[0].Description != "" {
		t.Fatalf("blank key matched: %#v", out.Objects)
	}
	dash := obj("-", 0x100, 1)
	moved := obj("-", 0x200, 1)
	moved.Description = "dash"
	out, _, err = Merge(&Model{Schema: SchemaID, Objects: []*Object{dash}}, &Model{Schema: SchemaID, Objects: []*Object{moved}}, MergeOptions{Description: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Objects) != 1 || out.Objects[0].Address != 0x100 || out.Objects[0].Description != "dash" {
		t.Fatalf("%#v", out.Objects[0])
	}
}

func TestMergeOrder(t *testing.T) {
	base := &Model{Schema: SchemaID, Objects: []*Object{obj("A", 0x1, 1), obj("B", 0x2, 1)}}
	inc := &Model{Schema: SchemaID, Objects: []*Object{obj("C", 0x3, 1), obj("D", 0x4, 1)}}
	out, _, err := Merge(base, inc, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, o := range out.Objects {
		keys = append(keys, o.Key)
	}
	if got, want := keys[0]+keys[1]+keys[2]+keys[3], "ABCD"; got != want {
		t.Fatal(got)
	}
}
