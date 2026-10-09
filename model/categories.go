package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// CategoryTableSchema is the schema version of a category table.
const CategoryTableSchema = 1

// CategoryTable is the corpus categories.json (docs/corpus.md): the tuner
// set of map names, each with its category.
type CategoryTable struct {
	Schema     int               `json:"schema" doc:"Schema version." enum:"1"`
	Categories map[string]string `json:"categories" doc:"Category of each map name in the tuner set."`
	sum        string
}

// ParseCategoryTable reads and checks a category table. Unknown keys are an
// error, as in categories.schema.json.
func ParseCategoryTable(b []byte) (*CategoryTable, error) {
	t := new(CategoryTable)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(t); err != nil {
		return nil, fmt.Errorf("category table: %w", err)
	}
	if t.Schema != CategoryTableSchema {
		return nil, fmt.Errorf("category table: schema %d, want %d", t.Schema, CategoryTableSchema)
	}
	if len(t.Categories) == 0 {
		return nil, fmt.Errorf("category table: no categories")
	}
	for n, c := range t.Categories {
		if n == "" || c == "" {
			return nil, fmt.Errorf("category table: empty name or category %q: %q", n, c)
		}
	}
	s := sha256.Sum256(b)
	t.sum = hex.EncodeToString(s[:])
	return t, nil
}

// Name is the map name an object is listed under: the first word of its ID.
func (o *Object) Name() string {
	f := strings.Fields(o.ID)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// Tuner keeps the objects whose name is in the table and the objects their
// image axes point at, filed as by Categorize, and marks the provenance as a
// tuner subset. The table must come from ParseCategoryTable.
func (m *Model) Tuner(t *CategoryTable) error {
	if m.Provenance == nil {
		return errors.New("a tuner subset needs the model's provenance")
	}
	if t.sum == "" {
		return errors.New("category table not read by ParseCategoryTable")
	}
	m.Categorize(t, "")
	m.Objects = slices.DeleteFunc(m.Objects, func(o *Object) bool { return len(o.Categories) == 0 })
	m.Provenance.Subset = &Subset{Kind: "tuner", Table: t.sum}
	return nil
}

// Categorize replaces the categories with the table's: an object whose name
// is in the table gets its category, and an object its image axes point at
// (at the axis address, or before its header) gets the category of the first
// such map. Every other object gets rest, or no category when rest is "".
func (m *Model) Categorize(t *CategoryTable, rest string) {
	cat := map[*Object]string{}
	at := map[Addr][]*Object{}
	for _, o := range m.Objects {
		at[o.Address] = append(at[o.Address], o)
		if c, ok := t.Categories[o.Name()]; ok {
			cat[o] = c
		}
	}
	for _, o := range m.Objects {
		c, ok := cat[o]
		if !ok {
			continue
		}
		for _, ax := range []*Axis{o.X, o.Y} {
			if ax == nil || ax.Address == nil {
				continue
			}
			addrs := []Addr{*ax.Address}
			if h := Addr(ax.Header); h > 0 && h <= *ax.Address {
				addrs = append(addrs, *ax.Address-h)
			}
			for _, a := range addrs {
				for _, r := range at[a] {
					if _, ok := cat[r]; !ok {
						cat[r] = c
					}
				}
			}
		}
	}
	if rest != "" {
		for _, o := range m.Objects {
			if _, ok := cat[o]; !ok {
				cat[o] = rest
			}
		}
	}
	var names []string
	for _, c := range cat {
		if !slices.Contains(names, c) {
			names = append(names, c)
		}
	}
	slices.Sort(names)
	m.Categories = make([]*Category, len(names))
	for i, n := range names {
		m.Categories[i] = &Category{ID: i, Name: n}
	}
	for _, o := range m.Objects {
		o.Categories = nil
		if c, ok := cat[o]; ok {
			o.Categories = []int{slices.Index(names, c)}
		}
	}
}
