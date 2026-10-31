package model

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/kp"
)

var update = flag.Bool("update", false, "rewrite schema.json and kp-defaults.json")

// packModels runs fn in parallel on every ecuxplot pack whose file name
// starts with prefix, with the KP bytes, the parsed file and the model's
// stamped JSON.
func packModels(t *testing.T, prefix string, fn func(t *testing.T, in []byte, f *kp.File, js []byte)) {
	for _, p := range testenv.Packs(t) {
		name := filepath.Base(p)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			m, err := ReadKP(in, name)
			if err != nil {
				t.Fatal(err)
			}
			js, err := canon.MarshalStamped(m, "xdfkit test")
			if err != nil {
				t.Fatal(err)
			}
			f, _ := kp.Parse(in)
			fn(t, in, f, js)
		})
	}
}

// TestKPRoundTrip: KP to model JSON to KP, with the original as template,
// gives the same file: the same decoded fields, and for v1 the same bytes.
func TestKPRoundTrip(t *testing.T) {
	packModels(t, "", func(t *testing.T, in []byte, f *kp.File, js []byte) {
		var m Model
		if err := canon.Unmarshal(js, &m); err != nil {
			t.Fatal(err)
		}
		g, err := m.KP(f)
		if err != nil {
			t.Fatal(err)
		}
		a, _ := canon.Marshal(f)
		b, _ := canon.Marshal(g)
		if !bytes.Equal(a, b) {
			t.Fatal("KP differs after the model round trip")
		}
		out, err := g.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if f.Layout() == kp.V1 && !bytes.Equal(out, in) {
			t.Error("v1 bytes differ")
		}
		if _, err := kp.Parse(out); err != nil {
			t.Errorf("re-encoded file: %v", err)
		}
	})
}

// TestKPNoTemplate: without a template, KP output parses back to the same
// model.
func TestKPNoTemplate(t *testing.T) {
	packModels(t, "", func(t *testing.T, _ []byte, _ *kp.File, js []byte) {
		var m Model
		if err := canon.Unmarshal(js, &m); err != nil {
			t.Fatal(err)
		}
		g, err := m.KP(nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := g.Encode()
		if err != nil {
			t.Fatal(err)
		}
		back, err := ReadKP(out, m.Provenance.File)
		if err != nil {
			t.Fatal(err)
		}
		back.Provenance = m.Provenance
		a, _ := canon.Marshal(&m)
		b, _ := canon.Marshal(back)
		if !bytes.Equal(a, b) {
			t.Fatal("model differs after KP output without a template")
		}
	})
}

// TestEdit: a model edit reaches the KP field, and nothing else changes.
func TestEdit(t *testing.T) {
	packModels(t, "8D0907551M.", func(t *testing.T, _ []byte, f *kp.File, js []byte) {
		var m Model
		if err := canon.Unmarshal(js, &m); err != nil {
			t.Fatal(err)
		}
		o := m.Objects[0]
		o.Value.Conversion.Factor *= 2
		o.Description += " edited"
		g, err := m.KP(f)
		if err != nil {
			t.Fatal(err)
		}
		f.Project.Maps[0].Value.Factor *= 2
		f.Project.Maps[0].Name += " edited"
		a, _ := canon.Marshal(f)
		b, _ := canon.Marshal(g)
		if !bytes.Equal(a, b) {
			t.Fatal("edited model gives a different KP than the same edit on the KP")
		}
	})
}

// TestKPDefaultsFile: the committed kp-defaults.json holds, per layout and
// record kind, each field's commonest residue over ecuxplot's packs (and a
// local OLS5 pack when present), where it is commoner than no residue and not
// 0 (go test ./model -run TestKPDefaultsFile -update rewrites it). Without the
// local OLS5 file, a committed "ols5" block is kept as-is.
func TestKPDefaultsFile(t *testing.T) {
	total := map[string]int{}
	seen := map[string]map[string]map[string]int{}
	add := func(kind string, r map[string]any) {
		total[kind]++
		if seen[kind] == nil {
			seen[kind] = map[string]map[string]int{}
		}
		for k, v := range r {
			b, _ := json.Marshal(v)
			if seen[kind][k] == nil {
				seen[kind][k] = map[string]int{}
			}
			seen[kind][k][string(b)]++
		}
	}
	survey := func(f *kp.File) {
		m, l := FromKP(f), f.Layout().DefaultsKey()+"/"
		for i, c := range m.Categories {
			r, _ := residue(f.Project.Folders[i], c.kpFolder())
			add(l+"folder", r)
		}
		for i, o := range m.Objects {
			r, _ := residue(f.Project.Maps[i], o.kpMap(i))
			for k, x := range map[string]*Axis{"x": o.X, "y": o.Y} {
				a, _ := r[k].(map[string]any)
				delete(r, k)
				add(l+axisKind(x), a)
			}
			add(l+"map", r)
		}
	}
	for _, p := range testenv.Packs(t) {
		in, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := kp.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		survey(f)
	}
	ols5KP := filepath.Join(testenv.ModuleRoot(), "testdata", "local", "8N0906018BC_0002_366366.kp")
	ols5Surveyed := false
	if in, err := os.ReadFile(ols5KP); err == nil {
		f, err := kp.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if f.Layout() != kp.OLS5 {
			t.Fatalf("%s: layout %v, want OLS5", ols5KP, f.Layout())
		}
		survey(f)
		ols5Surveyed = true
	}
	gen := map[string]map[string]map[string]json.RawMessage{}
	for kind, fields := range seen {
		l, rec, _ := strings.Cut(kind, "/")
		for k, vals := range fields {
			best, n, sum := "", 0, 0
			for v, c := range vals {
				sum += c
				if c > n || c == n && v < best {
					best, n = v, c
				}
			}
			if n <= total[kind]-sum || best == "0" {
				continue
			}
			if gen[l] == nil {
				gen[l] = map[string]map[string]json.RawMessage{}
			}
			if gen[l][rec] == nil {
				gen[l][rec] = map[string]json.RawMessage{}
			}
			var v any
			d := json.NewDecoder(strings.NewReader(best))
			d.UseNumber()
			if err := d.Decode(&v); err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(hexRound(v))
			if err != nil {
				t.Fatal(err)
			}
			gen[l][rec][k] = json.RawMessage(b)
		}
	}
	if !ols5Surveyed {
		var committed map[string]map[string]map[string]json.RawMessage
		if err := json.Unmarshal(kpDefaultsJSON, &committed); err != nil {
			t.Fatal(err)
		}
		if committed["ols5"] != nil {
			gen["ols5"] = committed["ols5"]
		}
	}
	b, err := json.MarshalIndent(gen, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *update {
		if err := os.WriteFile("kp-defaults.json", b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(b, kpDefaultsJSON) && !*update {
		t.Fatal("kp-defaults.json is stale: run make schema")
	}
}

func TestKeys(t *testing.T) {
	m := &Model{Objects: []*Object{{ID: "A"}, {ID: "-"}, {ID: "A"}, {ID: "-"}, {ID: " "}, {ID: "A#2"}}}
	m.AssignKeys()
	var keys []string
	for _, o := range m.Objects {
		keys = append(keys, o.Key)
	}
	if got, want := strings.Join(keys, ","), "A,-,A,-,,A#2"; got != want {
		t.Errorf("keys %s, want %s", got, want)
	}
	m.Schema = SchemaID
	if err := m.Check(); err != nil {
		t.Error(err)
	}
	m.Objects[0].Key = ""
	if err := m.Check(); err != nil {
		t.Error(err)
	}
}

func TestReference(t *testing.T) {
	sha := strings.Repeat("0", 64)
	for _, c := range []struct {
		origin string
		ref    Reference
		ok     bool
		agreed bool
	}{
		{"auto", Reference{Origin: "damos", SHA256: sha, Agree: [2]int{10, 10}}, true, true},
		{"auto", Reference{Origin: "manual", SHA256: sha, Agree: [2]int{9, 10}, Except: []string{"ESKONF"}}, true, true},
		{"auto", Reference{Origin: "a2l", SHA256: sha, Agree: [2]int{8, 10}, Except: []string{"ESKONF"}}, true, false},
		{"auto", Reference{Origin: "auto", SHA256: sha}, false, false},
		{"auto", Reference{Origin: "damos", SHA256: sha, Agree: [2]int{11, 10}}, false, true},
		{"damos", Reference{Origin: "damos", SHA256: sha, Agree: [2]int{1, 1}}, false, true},
	} {
		m := &Model{Schema: SchemaID, Provenance: &Provenance{Format: "xdf", Origin: c.origin, Reference: &c.ref}}
		if err := m.Check(); (err == nil) != c.ok {
			t.Errorf("%s %+v: Check %v", c.origin, c.ref, err)
		}
		if c.ref.Agreed() != c.agreed {
			t.Errorf("%+v: Agreed %v", c.ref, !c.agreed)
		}
	}
	if (*Reference)(nil).Agreed() {
		t.Error("nil Agreed")
	}
}

func TestAddr(t *testing.T) {
	var a Addr
	for _, bad := range []string{`"13D52"`, `"0x"`, `"0x+1"`, `"0x100000000"`, `81234`} {
		if err := json.Unmarshal([]byte(bad), &a); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	if err := json.Unmarshal([]byte(`"0x13d52"`), &a); err != nil || a != 0x13D52 {
		t.Errorf("0x13d52: %v %x", err, a)
	}
	if b, _ := json.Marshal(a); string(b) != `"0x13D52"` {
		t.Errorf("marshal: %s", b)
	}
}

// TestSchemaFile: the committed schema.json and categories.schema.json are
// what Schema and CategoryTableJSONSchema generate
// (go test ./model -run TestSchemaFile -update rewrites them).
func TestSchemaFile(t *testing.T) {
	for file, fn := range map[string]func() ([]byte, error){
		"schema.json":            Schema,
		"categories.schema.json": CategoryTableJSONSchema,
	} {
		gen, err := fn()
		if err != nil {
			t.Fatal(err)
		}
		if *update {
			if err := os.WriteFile(file, gen, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		committed, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(committed, gen) {
			t.Errorf("%s is stale (%v): run make schema", file, err)
		}
	}
}

// TestSchemaValidates: every pack's model JSON is valid under schema.json.
func TestSchemaValidates(t *testing.T) {
	c := jsonschema.NewCompiler()
	sch, err := c.Compile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	packModels(t, "", func(t *testing.T, _ []byte, _ *kp.File, js []byte) {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
		if err != nil {
			t.Fatal(err)
		}
		if err := sch.Validate(doc); err != nil {
			t.Error(err)
		}
	})
}

// TestOmittedBase writes an omitted display base as 10, keeps an explicit
// base, and gives an axis with no storage a real width.
func TestOmittedBase(t *testing.T) {
	m := &Model{Schema: SchemaID, Objects: []*Object{{
		Key: "A", ID: "A", Shape: "2d", Address: 0x100, Rows: 2, Cols: 2,
		Data:  Data{Bits: 16, Endian: "little"},
		Value: Value{Conversion: Conversion{Factor: 1}},
		X:     &Axis{Source: "unknown", Value: Value{Conversion: Conversion{Factor: 1}}},
		Y:     &Axis{Source: "ordinal", View: View{Base: 16}, Value: Value{Conversion: Conversion{Factor: 1}}},
	}}}
	f, err := m.KP(nil)
	if err != nil {
		t.Fatal(err)
	}
	mp := f.Project.Maps[0]
	if mp.Base != 10 || mp.X.Base != 10 {
		t.Fatalf("base map %d x %d", mp.Base, mp.X.Base)
	}
	if mp.X.Type != 1 || mp.X.Width != 1 || mp.X.DataSource != kp.DSOrdinal {
		t.Fatalf("x type %s width %d source %s", mp.X.Type, mp.X.Width, mp.X.DataSource)
	}
	if mp.Y.Base != 16 || mp.Y.Width != 1 {
		t.Fatalf("y base %d width %d", mp.Y.Base, mp.Y.Width)
	}
}

// TestKPRange: without model limits the KP width pair is the default raw
// span; display min/max convert to raw (and order low to high).
func TestKPRange(t *testing.T) {
	lo, hi := 1500.0, 250.0 // display; factor 0.25 → raw 6000, 1000
	rlo, rhi := 10.0, 90.0  // display with factor 1 → raw 10, 90
	cell := func(key string, bits int, v Value) *Object {
		return &Object{
			Key: key, ID: key, Shape: "value", Address: 0x1000, Rows: 1, Cols: 1,
			Data: Data{Bits: bits}, Value: v,
		}
	}
	m := &Model{Schema: SchemaID, Objects: []*Object{
		cell("SCALE", 8, Value{Conversion: Conversion{Factor: 0.75}}),
		cell("LIMITS", 16, Value{Min: &lo, Max: &hi, Conversion: Conversion{Factor: 0.25}}),
		cell("SWAP", 8, Value{Min: &rhi, Max: &rlo, Conversion: Conversion{Factor: 1}}),
		cell("ID", 16, Value{Conversion: Conversion{Factor: 1}}),
	}}
	f, err := m.KP(nil)
	if err != nil {
		t.Fatal(err)
	}
	rng := func(i, slot int) [2]int64 { return f.Project.Maps[i].Range[slot] }
	if rng(0, 0) != [2]int64{0, 255} || rng(1, 1) != [2]int64{1000, 6000} || rng(2, 0) != [2]int64{10, 90} || rng(3, 1) != [2]int64{0, 65535} {
		t.Fatalf("%v %v %v %v", rng(0, 0), rng(1, 1), rng(2, 0), rng(3, 1))
	}
	back := FromKP(f)
	if back.Objects[0].Value.Min != nil || back.Objects[3].Value.Min != nil {
		t.Fatal("default raw span should omit model limits")
	}
	if back.Objects[1].Value.Min == nil || *back.Objects[1].Value.Min != 250 || *back.Objects[1].Value.Max != 1500 {
		t.Fatalf("16-bit limits: %#v", back.Objects[1].Value)
	}
}

// TestKPEditableValues round-trips free-editable axis values (8-bit and 16-bit).
func TestKPEditableValues(t *testing.T) {
	vals8 := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	vals16 := []float64{1, 2, 4, 5, 7, 8, 10, 11, 13, 14, 16}
	d8 := Data{Bits: 8}
	d16 := Data{Bits: 16, Endian: "little"}
	m := &Model{Schema: SchemaID, Objects: []*Object{
		{
			Key: "T8", ID: "T8", Shape: "1d", Address: 0x100, Rows: 1, Cols: 8,
			Data: d8, Value: Value{Conversion: Conversion{Factor: 1}},
			X: &Axis{Source: "editable", Data: &d8, Value: Value{Conversion: Conversion{Factor: 1}, Precision: 1}, Values: vals8},
		},
		{
			Key: "T16", ID: "T16", Shape: "2d", Inverse: true, Address: 0x200, Rows: 16, Cols: 11,
			Data: d16, Value: Value{Conversion: Conversion{Factor: 1}},
			X: &Axis{Source: "editable", Data: &d16, Value: Value{Conversion: Conversion{Factor: 1}, Precision: 4}, Values: vals16},
			Y: &Axis{Source: "ordinal", Value: Value{Conversion: Conversion{Factor: 1}}},
		},
	}}
	f, err := m.KP(nil)
	if err != nil {
		t.Fatal(err)
	}
	x0 := f.Project.Maps[0].X
	if x0.DataSource != kp.DSFreeEditable || len(x0.Values) != 32 || string(x0.Values[:8]) != "\x01\x02\x03\x04\x05\x06\x07\x08" {
		t.Fatalf("8-bit pack: ds=%v len=%d %x", x0.DataSource, len(x0.Values), x0.Values)
	}
	x1 := f.Project.Maps[1].X
	if x1.DataSource != kp.DSFreeEditable || len(x1.Values) != 64 {
		t.Fatalf("16-bit pack: ds=%v len=%d", x1.DataSource, len(x1.Values))
	}
	back := FromKP(f)
	if !equalFloats(back.Objects[0].X.Values, vals8) || !equalFloats(back.Objects[1].X.Values, vals16) {
		t.Fatalf("round trip values: %#v %#v", back.Objects[0].X.Values, back.Objects[1].X.Values)
	}
}

func equalFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
