package lint

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/kp"
	"go.nyet.org/xdfkit/model"
)

type fixtureAxis struct {
	Addr   model.Addr `json:"addr"`
	Bits   int        `json:"bits"`
	Stored string     `json:"stored"`
	Count  *int       `json:"count"`
	Values []uint32   `json:"values"`
}

type fixtureMap struct {
	ID   string       `json:"id"`
	Addr model.Addr   `json:"addr"`
	End  *model.Addr  `json:"end"`
	X    *fixtureAxis `json:"x"`
	Y    *fixtureAxis `json:"y"`
}

type fixtureCase struct {
	Name  string       `json:"name"`
	Ident string       `json:"ident"`
	Maps  []fixtureMap `json:"maps"`
	Want  []struct {
		ID         string   `json:"id"`
		Confidence string   `json:"confidence"`
		Maps       []string `json:"maps"`
	} `json:"want"`
}

const fixtureImageSize = 0x10000

// build writes the case's ident and axis values into an image and converts
// its maps to a KP file through the model, as xdfkit would.
func (c *fixtureCase) build(t *testing.T) (*kp.File, []byte) {
	img := make([]byte, fixtureImageSize)
	copy(img[0x100:], c.Ident)
	m := &model.Model{Schema: model.SchemaID}
	axis := func(a *fixtureAxis) (*model.Axis, int) {
		if a == nil {
			return nil, 1
		}
		if a.Bits == 0 {
			a.Bits = 8
		}
		d := model.Data{Bits: a.Bits}
		w := a.Bits / 8
		for i, v := range a.Values {
			p := img[int(a.Addr)+i*w:]
			switch w {
			case 1:
				p[0] = byte(v)
			case 2:
				d.Endian = "big"
				binary.BigEndian.PutUint16(p, uint16(v))
			}
		}
		if w == 1 {
			n := len(a.Values)
			if a.Count != nil {
				n = *a.Count
			}
			img[a.Addr-1] = byte(n)
		}
		stored := a.Stored
		if stored == "" {
			stored = "absolute"
		}
		addr := a.Addr
		return &model.Axis{Source: "image", Stored: stored, Address: &addr, Data: &d, Value: model.Value{Conversion: model.Conversion{Factor: 1}}}, len(a.Values)
	}
	for _, fm := range c.Maps {
		x, cols := axis(fm.X)
		y, rows := axis(fm.Y)
		shape := "1d"
		if y != nil {
			shape = "2d"
		}
		m.Objects = append(m.Objects, &model.Object{
			ID: fm.ID, Shape: shape, Address: fm.Addr, Rows: rows, Cols: cols,
			Data: model.Data{Bits: 8}, Value: model.Value{Conversion: model.Conversion{Factor: 1}}, X: x, Y: y,
		})
	}
	m.AssignKeys()
	f, err := m.KP(nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, fm := range c.Maps {
		if fm.End != nil {
			f.Project.Maps[i].End = uint32(*fm.End)
		}
	}
	return f, img
}

func fixtureCases(t *testing.T) []fixtureCase {
	b, err := os.ReadFile(filepath.Join(testenv.ModuleRoot(), "testdata", "lint", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Cases []fixtureCase }
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Cases
}

// TestFixtures runs every rule on synthetic images, so lint changes can be
// checked without the corpus. It then applies every fix, re-encodes, and
// checks that only report-only findings are left.
func TestFixtures(t *testing.T) {
	for _, c := range fixtureCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			f, img := c.build(t)
			r, err := Lint(f, img, Options{})
			if err != nil {
				t.Fatal(err)
			}
			got, want := map[string]string{}, map[string]string{}
			gotMaps, wantMaps := map[string][]string{}, map[string][]string{}
			for _, fd := range r.Findings {
				got[fd.ID] = fd.Confidence
				for _, ref := range fd.Maps {
					gotMaps[fd.ID] = append(gotMaps[fd.ID], ref.ID+"/"+ref.Axis)
				}
			}
			for _, w := range c.Want {
				want[w.ID] = w.Confidence
				wantMaps[w.ID] = w.Maps
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("findings %v, want %v", got, want)
			}
			if !reflect.DeepEqual(gotMaps, wantMaps) {
				t.Errorf("maps %v, want %v", gotMaps, wantMaps)
			}

			if err := Apply(f, Select(r.Findings, nil, nil, Low)); err != nil {
				t.Fatal(err)
			}
			out, err := f.Encode()
			if err != nil {
				t.Fatal(err)
			}
			g, err := kp.Parse(out)
			if err != nil {
				t.Fatal(err)
			}
			r2, err := Lint(g, img, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, fd := range r2.Findings {
				if fd.Fix != nil {
					t.Errorf("%s left after fixing", fd.ID)
				}
			}
		})
	}
}
