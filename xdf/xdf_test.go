package xdf

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/internal/corpus"
	"go.nyet.org/xdfkit/internal/testenv"
	"go.nyet.org/xdfkit/model"
)

type node struct {
	Name  string
	Attrs map[string]string
	Text  string
	Kids  []*node
}

func (n *node) String() string {
	keys := slices.Sorted(maps.Keys(n.Attrs))
	var a []string
	for _, k := range keys {
		a = append(a, fmt.Sprintf("%s=%q", k, n.Attrs[k]))
	}
	return fmt.Sprintf("<%s %s>%q", n.Name, strings.Join(a, " "), n.Text)
}

// parse reads an XML document into a tree, ignoring comments and the
// whitespace around elements.
func parse(t *testing.T, b []byte) *node {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(b))
	root := &node{}
	stack := []*node{root}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		top := stack[len(stack)-1]
		switch tok := tok.(type) {
		case xml.StartElement:
			n := &node{Name: tok.Name.Local, Attrs: map[string]string{}}
			for _, a := range tok.Attr {
				n.Attrs[a.Name.Local] = a.Value
			}
			top.Kids = append(top.Kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			top.Text += strings.TrimSpace(string(tok))
		}
	}
	return root
}

// diff reports where got and want differ, up to max lines.
func diff(path string, got, want *node, out *[]string, limit int) {
	if len(*out) >= limit {
		return
	}
	if got.String() != want.String() {
		*out = append(*out, fmt.Sprintf("%s: got %s, want %s", path, got, want))
		return
	}
	// mapdump wrote text that was only whitespace as empty elements
	want.Kids = slices.DeleteFunc(slices.Clone(want.Kids), func(n *node) bool {
		return (n.Name == "description" || n.Name == "units" || n.Name == "title") && n.Text == ""
	})
	if want.Name == "XDFAXIS" && !slices.ContainsFunc(got.Kids, isUnits) {
		// mapdump wrote the KP's units on axes the model doesn't have
		want.Kids = slices.DeleteFunc(slices.Clone(want.Kids), func(n *node) bool { return isUnits(n) && n.Text == "-" })
	}
	for i := range max(len(got.Kids), len(want.Kids)) {
		p := fmt.Sprintf("%s/%d", path, i)
		switch {
		case i >= len(got.Kids):
			*out = append(*out, fmt.Sprintf("%s: missing %s", p, want.Kids[i]))
		case i >= len(want.Kids):
			*out = append(*out, fmt.Sprintf("%s: extra %s", p, got.Kids[i]))
		default:
			diff(p+"("+want.Kids[i].Name+")", got.Kids[i], want.Kids[i], out, limit)
			continue
		}
		if len(*out) >= limit {
			return
		}
	}
}

func isUnits(n *node) bool { return n.Name == "units" }

func readModel(t *testing.T, dir, stem string) *model.Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, stem+".kp"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.ReadKP(b, stem+".kp")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestArchive writes every archived pack: the XDF must parse, be ASCII, and
// hold one table or constant per object.
func TestArchive(t *testing.T) {
	dir := testenv.Archive(t)
	kps, _ := filepath.Glob(filepath.Join(dir, "*.kp"))
	for _, path := range kps {
		stem := strings.TrimSuffix(filepath.Base(path), ".kp")
		t.Run(stem, func(t *testing.T) {
			m := readModel(t, dir, stem)
			var img []byte
			if !strings.Contains(stem, "-") && stem != "test8maps" {
				img = corpus.ArchiveImage(t, stem)
			}
			b, err := Write(m, img, stem)
			if err != nil {
				t.Fatal(err)
			}
			if i := bytes.IndexFunc(b, func(r rune) bool { return r >= 0x80 }); i >= 0 {
				t.Errorf("non-ASCII at %d", i)
			}
			root := parse(t, b)
			if len(root.Kids) != 1 || root.Kids[0].Name != "XDFFORMAT" {
				t.Fatalf("root %v", root.Kids)
			}
			if n := len(root.Kids[0].Kids) - 1; n != len(m.Objects) {
				t.Errorf("%d objects, want %d", n, len(m.Objects))
			}
		})
	}
}

func TestJavaFixed(t *testing.T) {
	for _, c := range []struct {
		f    float64
		prec int
		want string
	}{
		{0, 0, "0"}, {0, 2, "0.00"}, {2.5, 0, "3"}, {0.125, 2, "0.13"}, {0.15, 1, "0.2"},
		{1.005, 2, "1.01"}, {9.995, 2, "10.00"}, {0.004, 2, "0.00"}, {0.0004, 2, "0.00"},
		{0.6, 0, "1"}, {-1.5, 0, "-2"}, {255, 6, "255.000000"}, {1e7, 1, "10000000.0"},
		{-0.001, 2, "-0.00"}, {123.456, 1, "123.5"},
	} {
		if got := javaFixed(c.f, c.prec); got != c.want {
			t.Errorf("javaFixed(%v, %d) = %q, want %q", c.f, c.prec, got, c.want)
		}
	}
}

func TestLimitPrecision(t *testing.T) {
	v := func(prec int, factor float64) model.Value {
		return model.Value{Precision: prec, Conversion: model.Conversion{Factor: factor}}
	}
	for _, c := range []struct {
		v      model.Value
		bits   int
		signed bool
		want   int
	}{
		{v(2, 1), 8, false, 2},      // 255: 3 + 2 digits
		{v(4, 1), 8, false, 3},      // 255: 3 + 4 digits, cut to 3
		{v(4, 1), 16, false, 1},     // 65535
		{v(4, 1), 16, true, 1},      // 32767
		{v(9, 0.001), 16, false, 4}, // 65.535
		{v(4, 1), 32, false, 4},     // Java's 1<<32 == 1: raw 0
		{v(4, 1), 0, false, 4},      // no storage: raw 0
		{v(4, 100), 16, false, 0},   // 6553500
	} {
		if got := limitPrecision(c.v, c.bits, c.signed); got != c.want {
			t.Errorf("limitPrecision(%+v, %d, %v) = %d, want %d", c.v, c.bits, c.signed, got, c.want)
		}
	}
}

// TestMapdump compares the XDF of each archived pack with mapdump's XDF of
// the KP that xdfkit writes from the model (make -C publish), in the directory
// XDFKIT_MAPDUMP_DIR.
func TestMapdump(t *testing.T) {
	ref := os.Getenv("XDFKIT_MAPDUMP_DIR")
	if ref == "" {
		t.Skip("XDFKIT_MAPDUMP_DIR not set")
	}
	dir := testenv.Archive(t)
	xdfs, _ := filepath.Glob(filepath.Join(ref, "*.xdf"))
	for _, path := range xdfs {
		stem := strings.TrimSuffix(filepath.Base(path), ".xdf")
		t.Run(stem, func(t *testing.T) {
			got, err := Write(readModel(t, dir, stem), corpus.ArchiveImage(t, stem), stem)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var out []string
			diff("", parse(t, got), parse(t, want), &out, 20)
			for _, s := range out {
				t.Error(s)
			}
		})
	}
}
