package xdf

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"
)

func valueAt(key string, addr model.Addr, cat int, units string, factor float64) *model.Object {
	return &model.Object{
		Key: key, ID: key, Shape: "value", Address: addr, Rows: 1, Cols: 1,
		Categories: []int{cat}, Data: model.Data{Bits: 8},
		Value: model.Value{Units: units, Conversion: model.Conversion{Factor: factor}},
	}
}

func TestFoldRoundTrip(t *testing.T) {
	fuel := &model.Category{ID: 0, Name: "Fuel"}
	boost := &model.Category{ID: 1, Name: "Boost"}
	a := valueAt("KFZW", 0x100, 0, "deg", 0.75)
	b := valueAt("KFZW", 0x100, 1, "deg", 0.75)
	m := &model.Model{Schema: model.SchemaID, Categories: []*model.Category{fuel, boost}, Objects: []*model.Object{a, b}}
	folded, _, err := model.Merge(m, nil, model.MergeOptions{Smart: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(folded.Objects) != 1 || len(folded.Objects[0].Categories) != 2 {
		t.Fatalf("fold %#v", folded.Objects)
	}
	x, err := Write(folded, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(x), "<XDFCONSTANT ") != 2 || !strings.Contains(string(x), foldPrefix) {
		t.Fatalf("expansion missing in %s", x)
	}
	meta, err := Meta(folded, x, "test")
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Read(x, meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Objects) != 1 || len(back.Objects[0].Categories) != 2 {
		t.Fatalf("read %#v", back.Objects)
	}

	deg := valueAt("LDR", 0x400, 0, "deg", 0.75)
	afr := valueAt("LDR", 0x400, 0, "afr", 2)
	m = &model.Model{Schema: model.SchemaID, Categories: []*model.Category{fuel}, Objects: []*model.Object{deg, afr}}
	folded, _, err = model.Merge(m, nil, model.MergeOptions{Smart: true})
	if err != nil {
		t.Fatal(err)
	}
	x, err = Write(folded, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	meta, err = Meta(folded, x, "test")
	if err != nil {
		t.Fatal(err)
	}
	back, _, err = Read(x, meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Objects) != 1 || len(back.Objects[0].Alternates) != 1 || back.Objects[0].Alternates[0].Value.Units != "afr" {
		t.Fatalf("alternate %#v", back.Objects[0])
	}
}

func TestMinMaxRoundTrip(t *testing.T) {
	lo, hi := 1.25, 3.5
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{{
		Key: "M", ID: "M", Shape: "1d", Address: 0x100, Rows: 1, Cols: 4,
		Data:  model.Data{Bits: 8},
		Value: model.Value{Conversion: model.Conversion{Factor: 1}, Min: &lo, Max: &hi},
	}}}
	x, err := Write(m, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(x), "<min>1.250000</min>") || !strings.Contains(string(x), "<max>3.500000</max>") {
		t.Fatalf("limits %s", x)
	}
	plain, _, err := Read(x, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Objects[0].Value.Min == nil || *plain.Objects[0].Value.Min != lo || plain.Objects[0].Value.Max == nil || *plain.Objects[0].Value.Max != hi {
		t.Fatalf("plain %#v", plain.Objects[0].Value)
	}
	meta, err := Meta(m, x, "test")
	if err != nil {
		t.Fatal(err)
	}
	back, _, err := Read(x, meta)
	if err != nil {
		t.Fatal(err)
	}
	if back.Objects[0].Value.Min == nil || *back.Objects[0].Value.Min != lo || *back.Objects[0].Value.Max != hi {
		t.Fatalf("meta %#v", back.Objects[0].Value)
	}
}
