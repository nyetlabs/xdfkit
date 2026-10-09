package xdf

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/model"
)

func marshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := canon.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// firstDiff returns the first differing line of a and b, for error messages.
func firstDiff(a, b []byte) string {
	al, bl := strings.Split(string(a), "\n"), strings.Split(string(b), "\n")
	for i := range min(len(al), len(bl)) {
		if al[i] != bl[i] {
			return "line " + al[i] + " | want " + bl[i]
		}
	}
	return "lengths differ"
}

// TestMetaRoundTrip writes every archived pack as XDF plus metadata file,
// with and without its image: reading both back must give the model, and the
// XDF alone a valid model.
func TestMetaRoundTrip(t *testing.T) {
	dir := testenv.Archive(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, path := range kps {
		stem := strings.TrimSuffix(filepath.Base(path), ".kp")
		t.Run(stem, func(t *testing.T) {
			m := readModel(t, dir, stem)
			want := marshal(t, m)
			imgs := [][]byte{nil}
			if !strings.Contains(stem, "-") && stem != "test8maps" {
				imgs = append(imgs, corpus.ArchiveImage(t, stem))
			}
			for _, img := range imgs {
				x, err := Write(m, img, stem)
				if err != nil {
					t.Fatal(err)
				}
				meta, err := Meta(m, x, "xdfkit test")
				if err != nil {
					t.Fatal(err)
				}
				if s, _, err := canon.Verify(meta); err != nil || s != canon.Clean {
					t.Errorf("metadata stamp %v, %v", s, err)
				}
				got, ws, err := Read(x, meta)
				if err != nil {
					t.Fatal(err)
				}
				for _, w := range ws {
					t.Errorf("warning: %s", w)
				}
				if b := marshal(t, got); !bytes.Equal(b, want) {
					t.Errorf("image %v: XDF plus metadata differs from the model: %s", img != nil, firstDiff(b, want))
				}
				// a wrong XDF digest merges every object as possibly edited,
				// which must still keep the unedited ones
				i := bytes.LastIndex(meta, []byte(`"sha256:`)) + len(`"sha256:`)
				wrong := slices.Clone(meta)
				wrong[i] = map[bool]byte{true: '1', false: '0'}[wrong[i] == '0']
				got, ws, err = Read(x, wrong)
				if err != nil {
					t.Fatal(err)
				}
				if len(ws) != 2 || !strings.Contains(ws[1], "XDF edited") {
					t.Errorf("wrong digest: warnings %q", ws[:min(len(ws), 5)])
				}
				if b := marshal(t, got); !bytes.Equal(b, want) {
					t.Errorf("image %v: wrong digest: differs from the model: %s", img != nil, firstDiff(b, want))
				}
				v, _, err := Read(x, nil)
				if err != nil {
					t.Fatal(err)
				}
				if err := v.Check(); err != nil {
					t.Error(err)
				}
				if len(v.Objects) != len(m.Objects) {
					t.Errorf("view has %d objects, want %d", len(v.Objects), len(m.Objects))
				}
			}
		})
	}
}

// TestMetaEdit renames and edits a table, deletes one and adds one in an
// XDF: the edits are kept and every other object comes back unchanged.
func TestMetaEdit(t *testing.T) {
	dir := testenv.Archive(t)
	m := readModel(t, dir, "8D0907551M")
	x, err := Write(m, nil, "8D0907551M")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := Meta(m, x, "xdfkit test")
	if err != nil {
		t.Fatal(err)
	}
	s := string(x)
	// element returns the span of the object with uniqueid uid, which is
	// m.Objects[uid-1].
	element := func(uid string) (int, int) {
		i := strings.Index(s, `uniqueid="`+uid+`"`)
		if i < 0 {
			t.Fatalf("%s not found", uid)
		}
		i = strings.LastIndex(s[:i], "<XDF")
		elem, _, _ := strings.Cut(s[i+1:], " ")
		return i, i + strings.Index(s[i:], "</"+elem+">") + len(elem) + 3
	}
	// 0x1's title, KFLDRQ2, repeats at 0x168: renaming it must not change
	// 0x168's key
	i, j := element("0x1")
	s = s[:i] + strings.Replace(s[i:j], "<title>KFLDRQ2<", "<title>EDITED<", 1) + s[j:]
	i, j = element("0x3")
	if m.Objects[2].Shape == "value" {
		t.Fatal("0x3 is a constant; pick a table")
	}
	z := i + strings.Index(s[i:j], `<XDFAXIS id="z">`)
	s = s[:z] + strings.Replace(s[z:j], `<MATH equation="`, `<MATH equation="2 * `, 1) + s[j:]
	// delete 0x4, add a copy of it as 0xFFFF
	i, j = element("0x4")
	moved := strings.Replace(s[i:j], `uniqueid="0x4"`, `uniqueid="0xFFFF"`, 1)
	s = s[:i] + s[j:]
	s = strings.Replace(s, "</XDFFORMAT>", moved+"\n</XDFFORMAT>", 1)

	got, ws, err := Read([]byte(s), meta)
	if err != nil {
		t.Fatal(err)
	}
	// 0x3's new factor is not in its metadata, so nothing of it is dropped
	if len(ws) != 4 || !strings.HasPrefix(ws[0], "XDF edited") ||
		ws[1] != `0x1 "EDITED": metadata not applied to id` ||
		!strings.HasPrefix(ws[2], "0xFFFF ") || !strings.HasPrefix(ws[3], "0x4: ") {
		t.Errorf("warnings %q", ws)
	}
	// 0x4 is gone and its copy, read without metadata, comes last
	want := append(slices.Clone(m.Objects[:3]), m.Objects[4:]...)
	if len(got.Objects) != len(want)+1 {
		t.Fatalf("%d objects, want %d", len(got.Objects), len(want)+1)
	}
	for i, w := range want {
		g := got.Objects[i]
		c := *g
		switch i {
		case 0:
			if g.ID != "EDITED" {
				t.Errorf("renamed object %s: id %q", g.Key, g.ID)
			}
			c.ID, c.Description, c.Comment = w.ID, w.Description, w.Comment
		case 2:
			if g.Value.Conversion.Factor != 2*w.Value.Conversion.Factor {
				t.Errorf("edited object %s: factor %v, want %v", g.Key, g.Value.Conversion.Factor, 2*w.Value.Conversion.Factor)
			}
			c.Value.Conversion = w.Value.Conversion
		}
		g = &c
		gb, wb := marshal(t, g), marshal(t, w)
		if !bytes.Equal(gb, wb) {
			t.Errorf("object %d %s changed: %s", i, g.Key, firstDiff(gb, wb))
		}
	}
}

func TestParseEquation(t *testing.T) {
	for _, c := range []struct {
		s    string
		want model.Conversion
		bad  bool
	}{
		{"X", model.Conversion{Factor: 1}, false},
		{"", model.Conversion{Factor: 1}, false},
		{"0.75 * X", model.Conversion{Factor: 0.75}, false},
		{"0.75 * X+ -48", model.Conversion{Factor: 0.75, Offset: -48}, false},
		{"X*0.75-48", model.Conversion{Factor: 0.75, Offset: -48}, false},
		{"(X-128)/2", model.Conversion{Factor: 0.5, Offset: -64}, false},
		{"-2 * X", model.Conversion{Factor: -2}, false},
		{"1000 / X+ 3", model.Conversion{Factor: 1000, Offset: 3, Reciprocal: true}, false},
		{"5.96E-8 * X", model.Conversion{Factor: 5.96e-8}, false},
		{"X*X", model.Conversion{}, true},
		{"5", model.Conversion{}, true},
		{"X + 1/X", model.Conversion{}, true},
		{"2 * (X", model.Conversion{}, true},
		{"Y", model.Conversion{}, true},
	} {
		got, err := parseEquation(c.s)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("parseEquation(%q) = %+v, %v", c.s, got, err)
		}
	}
}
