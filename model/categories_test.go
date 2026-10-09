package model

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"go.nyet.org/xdfkit/internal/corpus"
)

func TestParseCategoryTable(t *testing.T) {
	for _, s := range []string{
		`{"schema": 2, "categories": {"KFZW": "Timing"}}`,
		`{"schema": 1, "categories": {}}`,
		`{"schema": 1, "categories": {"KFZW": ""}}`,
		`{"schema": 1, "categories": {"KFZW": "Timing"}, "extra": 1}`,
		`not json`,
	} {
		if _, err := ParseCategoryTable([]byte(s)); err == nil {
			t.Errorf("%s: no error", s)
		}
	}
	if _, err := ParseCategoryTable([]byte(`{"schema": 1, "categories": {"KFZW": "Timing"}}`)); err != nil {
		t.Error(err)
	}
}

// TestTuner: listed maps and the objects their axes point at (exactly, or
// before the axis header, which must not wrap below 0) are kept with the
// table's categories, and the provenance is marked as a subset.
func TestTuner(t *testing.T) {
	addr := func(a Addr) *Addr { return &a }
	m := &Model{
		Provenance: &Provenance{Format: "kp", SHA256: strings.Repeat("0", 64)},
		Categories: []*Category{{ID: 0, Name: "My maps"}},
		Objects: []*Object{
			{Key: "KFZW", ID: "KFZW", Categories: []int{0}, Address: 0x300,
				X: &Axis{Address: addr(0x100)}, Y: &Axis{Address: addr(0x202), Header: 2}},
			{Key: "SNM12ZWUW", ID: "SNM12ZWUW", Address: 0x100},
			{Key: "SNM16ZWUB", ID: "SNM16ZWUB", Address: 0x200},
			{Key: "KFMIOP", ID: "KFMIOP map", Address: 0x400, X: &Axis{Address: addr(0x1), Header: 2}},
			{Key: "WRAPPED", ID: "WRAPPED", Address: 0xFFFFFFFF},
			{Key: "UNLISTED", ID: "UNLISTED", Address: 0x500},
		},
	}
	tab, err := ParseCategoryTable([]byte(`{"schema": 1, "categories": {"KFZW": "Timing", "KFMIOP": "Torque", "ABSENT": "Boost"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := (&Model{}).Tuner(tab); err == nil {
		t.Error("no provenance: no error")
	}
	if err := m.Tuner(tab); err != nil {
		t.Fatal(err)
	}
	if s := m.Provenance.Subset; s == nil || s.Kind != "tuner" || len(s.Table) != 64 {
		t.Errorf("subset %+v", s)
	}
	var keys []string
	for _, o := range m.Objects {
		keys = append(keys, o.Key+"="+m.Categories[o.Categories[0]].Name)
	}
	want := []string{"KFZW=Timing", "SNM12ZWUW=Timing", "SNM16ZWUB=Timing", "KFMIOP=Torque"}
	if !slices.Equal(keys, want) {
		t.Errorf("got %v, want %v", keys, want)
	}
	if len(m.Categories) != 2 {
		t.Errorf("categories %v", m.Categories)
	}
}

// TestCorpusCategoryTable: the corpus categories.json is canonical, valid
// under categories.schema.json and loads.
func TestCorpusCategoryTable(t *testing.T) {
	c := corpus.Open(t)
	b, err := os.ReadFile(c.Dir + "/categories.json")
	if err != nil {
		t.Skip(err)
	}
	sch, err := jsonschema.NewCompiler().Compile("categories.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(doc); err != nil {
		t.Error(err)
	}
	if _, err := ParseCategoryTable(b); err != nil {
		t.Error(err)
	}
}
