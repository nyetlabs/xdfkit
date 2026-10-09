// Package model is the canonical, format-neutral map definition model
// (docs/model.md). Readers convert into it and writers convert out of it; its
// JSON form, written through canon, is the interchange and corpus format,
// described by schema.json (generated from these types). It holds no
// format-specific residue: writers fill what the model lacks from defaults or
// a template file.
package model

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// SchemaID names the schema version every model document carries.
const SchemaID = "xdfkit-model/1"

// Model is one map definition file.
type Model struct {
	Schema     string      `json:"schema" doc:"Schema version." enum:"xdfkit-model/1"`
	Provenance *Provenance `json:"provenance,omitempty" doc:"Where the definition was converted from."`
	Project    Project     `json:"project,omitzero" doc:"Project metadata."`
	Categories []*Category `json:"categories,omitempty" doc:"Categories (KP folders, XDF categories), referenced by id from objects."`
	Objects    []*Object   `json:"objects" doc:"Map definitions, in source order."`
}

// Provenance identifies the original file, which the corpus doesn't hold.
type Provenance struct {
	Format string  `json:"format" doc:"Source format." enum:"kp|xdf"`
	Origin string  `json:"origin,omitempty" doc:"Where the definitions came from, whatever format carried them: damos or a2l (exported from Bosch data), hand (made by hand, as KP projects usually are)." enum:"damos|a2l|hand"`
	File   string  `json:"file,omitempty" doc:"Original file name."`
	SHA256 string  `json:"sha256" doc:"SHA-256 of the original file, lowercase hex." pattern:"^[0-9a-f]{64}$"`
	Subset *Subset `json:"subset,omitempty" doc:"Set when the definition is a subset of the original file's, which must not replace it in the corpus."`
}

// Subset says how a definition was cut down from the original's.
type Subset struct {
	Kind  string `json:"kind" doc:"tuner: the maps of a category table and their axes (Model.Tuner)." enum:"tuner"`
	Table string `json:"table" doc:"SHA-256 of the category table, lowercase hex." pattern:"^[0-9a-f]{64}$"`
}

// Origins are the values of Provenance.Origin.
var Origins = []string{"damos", "a2l", "hand"}

// Project is file-level metadata.
type Project struct {
	Name    string `json:"name,omitempty" doc:"Project name."`
	Version string `json:"version,omitempty" doc:"Project version."`
}

// Category is a named group of objects.
type Category struct {
	ID   int    `json:"id" doc:"Identifier referenced by objects."`
	Name string `json:"name" doc:"Display name."`
}

// Object is one map definition: a single value, a curve or a map.
type Object struct {
	Key         string `json:"key" doc:"Unique within the file: the source id, with #2, #3 appended to repeats in source order, or obj-N when the id is blank."`
	ID          string `json:"id" doc:"The source's own identifier (KP id, A2L name); not unique."`
	Description string `json:"description,omitempty" doc:"Long description (KP name)."`
	Comment     string `json:"comment,omitempty" doc:"Free-text comment."`
	Categories  []int  `json:"categories,omitempty" doc:"Category ids."`
	Shape       string `json:"shape" doc:"value: one cell; 1d: a row of cols cells; 2d: rows by cols cells." enum:"value|1d|2d"`
	Inverse     bool   `json:"inverse,omitzero" doc:"KP \"2d Inverse\" organisation; its meaning is unconfirmed (docs/kp-format.md)."`
	Address     Addr   `json:"address" doc:"File offset of the first cell."`
	Rows        int    `json:"rows" doc:"Number of rows."`
	Cols        int    `json:"cols" doc:"Number of columns."`
	Data        Data   `json:"data" doc:"Cell storage."`
	Value       Value  `json:"value" doc:"Cell meaning and conversion."`
	View        View   `json:"view,omitzero" doc:"Display settings."`
	X           *Axis  `json:"x,omitempty" doc:"Column axis."`
	Y           *Axis  `json:"y,omitempty" doc:"Row axis."`
}

// Data is how cells are stored.
type Data struct {
	Bits   int    `json:"bits" doc:"Cell width in bits." enum:"8|16|32"`
	Endian string `json:"endian,omitempty" doc:"Byte order of cells wider than 8 bits." enum:"big|little"`
	Signed bool   `json:"signed,omitzero" doc:"Two's complement integers."`
	Float  bool   `json:"float,omitzero" doc:"IEEE 754 floating point."`
}

// Value is what cells mean.
type Value struct {
	Description string     `json:"description,omitempty" doc:"Description of the quantity."`
	Units       string     `json:"units,omitempty" doc:"Physical units."`
	Precision   int        `json:"precision" doc:"Decimal places shown."`
	Conversion  Conversion `json:"conversion" doc:"Raw to physical conversion."`
}

// Conversion turns a raw cell into a physical value: factor * raw + offset.
type Conversion struct {
	Factor     float64 `json:"factor" doc:"Multiplier."`
	Offset     float64 `json:"offset" doc:"Added after the multiplication."`
	Reciprocal bool    `json:"reciprocal,omitzero" doc:"KP reciprocal flag; exact formula unconfirmed."`
}

// View holds display settings.
type View struct {
	Base       int  `json:"base,omitzero" doc:"Number base shown (KP: 10, 16 or 2)."`
	Difference bool `json:"difference,omitzero" doc:"Show the difference to the original."`
	Percent    bool `json:"percent,omitzero" doc:"Show the difference in percent."`
}

// Axis gives the breakpoints of a curve or map.
type Axis struct {
	Source    string `json:"source" doc:"image: read from the image; ordinal: 1, 2, 3; editable: fixed values (KP \"Free editable\", storage not located); unknown: an unnamed KP datasource (written back as ordinal)." enum:"image|ordinal|editable|unknown"`
	Stored    string `json:"stored,omitempty" doc:"How image values are stored: absolute; subtract: 2^bits minus the sum of the remaining raw values (docs/kp-format.md); add and backwards: semantics unknown." enum:"absolute|add|subtract|backwards"`
	Address   *Addr  `json:"address,omitempty" doc:"File offset of the first breakpoint, for image axes."`
	Data      *Data  `json:"data,omitempty" doc:"Breakpoint storage, for image axes."`
	Value     Value  `json:"value" doc:"Breakpoint meaning and conversion."`
	View      View   `json:"view,omitzero" doc:"Display settings."`
	Mirror    bool   `json:"mirror,omitzero" doc:"KP \"mirror map\" axis setting: WinOLS shows the axis in descending order; storage is unchanged."`
	Header    int    `json:"header,omitzero" doc:"Header bytes before the axis data (WinOLS script DataHeader; KP stores it, likely as the field after the signed flag); usually a point count."`
	Signature *int   `json:"signature,omitempty" doc:"Marker value WinOLS associates with the axis (WinOLS script SignaturByte, likely the byte before the axis data); omitted when none."`
}

// Addr is a file offset, written as a hex string ("0x13D52").
type Addr uint32

func (a Addr) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("0x%X", uint32(a))) }

func (a *Addr) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	h, ok := strings.CutPrefix(s, "0x")
	v, err := strconv.ParseUint(h, 16, 32)
	if !ok || err != nil || h == "" || h[0] == '+' {
		return fmt.Errorf("address %q is not a hex string like \"0x13D52\"", s)
	}
	*a = Addr(v)
	return nil
}

func (Addr) jsonSchema() map[string]any {
	return map[string]any{"type": "string", "pattern": "^0x[0-9A-Fa-f]{1,8}$"}
}

// AssignKeys sets the Key of each object that has none from its ID (see
// Object.Key), avoiding the keys already set.
func (m *Model) AssignKeys() {
	used := map[string]bool{}
	for _, o := range m.Objects {
		used[o.Key] = o.Key != ""
	}
	for i, o := range m.Objects {
		if o.Key != "" {
			continue
		}
		base := o.ID
		if strings.TrimSpace(base) == "" {
			base = fmt.Sprintf("obj-%d", i+1)
		}
		k := base
		for n := 2; used[k]; n++ {
			k = fmt.Sprintf("%s#%d", base, n)
		}
		used[k] = true
		o.Key = k
	}
}

// Check reports structural errors that the schema can't express.
func (m *Model) Check() error {
	if m.Schema != SchemaID {
		return fmt.Errorf("schema %q, want %q", m.Schema, SchemaID)
	}
	if p := m.Provenance; p != nil && p.Origin != "" && !slices.Contains(Origins, p.Origin) {
		return fmt.Errorf("provenance origin %q, want one of %s", p.Origin, strings.Join(Origins, ", "))
	}
	keys := map[string]bool{}
	for i, o := range m.Objects {
		if o == nil {
			return fmt.Errorf("objects[%d] is null", i)
		}
		if o.Key == "" || keys[o.Key] {
			return fmt.Errorf("objects[%d]: key %q is empty or repeated", i, o.Key)
		}
		keys[o.Key] = true
	}
	return nil
}
