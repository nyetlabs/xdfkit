package model

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
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
// record kind, each field's commonest residue over ecuxplot's packs, where it
// is commoner than no residue and not 0 (go test ./model -run TestKPDefaultsFile
// -update rewrites it).
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
	for _, p := range testenv.Packs(t) {
		in, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		f, err := kp.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		m, l := FromKP(f), fmt.Sprintf("v%d/", f.Layout())
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
	if got, want := strings.Join(keys, ","), "A,-,A#2,-#2,obj-5,A#2#2"; got != want {
		t.Errorf("keys %s, want %s", got, want)
	}
	m.Schema = SchemaID
	if err := m.Check(); err != nil {
		t.Error(err)
	}
	m.Objects[1].Key = "A"
	if err := m.Check(); err == nil {
		t.Error("repeated key accepted")
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
