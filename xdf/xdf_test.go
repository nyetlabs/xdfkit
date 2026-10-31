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

func liveNode(n *node) bool {
	for _, c := range n.Kids {
		if c.Name == "description" && strings.HasPrefix(c.Text, livePrefix) {
			return true
		}
	}
	return false
}

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

// maxAttr is the longest attribute value TunerPro reads: it copies each into
// a 256-byte buffer with strcpy_s and crashes on longer ones.
const maxAttr = 255

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
			n := 0
			for _, k := range root.Kids[0].Kids {
				if k.Name != "XDFHEADER" && !liveNode(k) {
					n++
				}
			}
			if n != len(m.Objects) {
				t.Errorf("%d objects, want %d", n, len(m.Objects))
			}
			var long func(*node)
			long = func(n *node) {
				for k, v := range n.Attrs {
					if len(v) > maxAttr {
						t.Errorf("%s %s: %d bytes, TunerPro reads at most %d", n.Name, k, len(v), maxAttr)
					}
				}
				for _, c := range n.Kids {
					long(c)
				}
			}
			long(root)
		})
	}
}

// TestVisLevel writes marked (1), unmarked (2) and an explicit higher level,
// reads them back, and keeps unmarked as model 0 through the metadata file.
func TestVisLevel(t *testing.T) {
	cell := func(id string, addr model.Addr, level int) *model.Object {
		return &model.Object{
			Key: id, ID: id, Shape: "value", Address: addr, Rows: 1, Cols: 1,
			Data: model.Data{Bits: 8}, Value: model.Value{Conversion: model.Conversion{Factor: 1}},
			VisLevel: level,
		}
	}
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{
		cell("MARKED", 0x10, 1),
		cell("PLAIN", 0x20, 0),
		cell("L3", 0x30, 3),
	}}
	x, err := Write(m, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	s := string(x)
	for _, want := range []string{
		`uniqueid="0x1" vislevel="1"`,
		`uniqueid="0x2" vislevel="2"`,
		`uniqueid="0x3" vislevel="3"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
	v, _, err := Read(x, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Objects[0].VisLevel != 1 || v.Objects[1].VisLevel != 2 || v.Objects[2].VisLevel != 3 {
		t.Errorf("view levels %d %d %d", v.Objects[0].VisLevel, v.Objects[1].VisLevel, v.Objects[2].VisLevel)
	}
	meta, err := Meta(m, x, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, ws, err := Read(x, meta)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range ws {
		t.Error(w)
	}
	if got.Objects[0].VisLevel != 1 || got.Objects[1].VisLevel != 0 || got.Objects[2].VisLevel != 3 {
		t.Errorf("with meta: levels %d %d %d", got.Objects[0].VisLevel, got.Objects[1].VisLevel, got.Objects[2].VisLevel)
	}
}

// TestVisLevelFromArchive: KP marks survive into XDF and back from the XDF
// alone (vislevel attribute), without a metadata file.
func TestVisLevelFromArchive(t *testing.T) {
	dir := testenv.Archive(t)
	m := readModel(t, dir, "4D1907558")
	var marked int
	for _, o := range m.Objects {
		if o.VisLevel == 1 {
			marked++
		}
	}
	if marked != 15 {
		t.Fatalf("archive model has %d vislevel 1, want 15", marked)
	}
	x, err := Write(m, nil, "4D1907558")
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := Read(x, nil)
	if err != nil {
		t.Fatal(err)
	}
	var n1, n2 int
	for _, o := range v.Objects {
		switch o.VisLevel {
		case 1:
			n1++
		case 2:
			n2++
		default:
			t.Errorf("%s: vislevel %d, want 1 or 2", o.Key, o.VisLevel)
		}
	}
	if n1 != marked {
		t.Errorf("XDF alone: %d vislevel 1, want %d", n1, marked)
	}
	if n2 != len(m.Objects)-marked {
		t.Errorf("XDF alone: %d vislevel 2, want %d", n2, len(m.Objects)-marked)
	}
}

// TestLinkedAxis: an image axis links to the curve at its address that holds
// its axis points (same count, storage and conversion up to KP rounding) and
// keeps its own location; any other axis stays embedded.
func TestLinkedAxis(t *testing.T) {
	addr := func(a model.Addr) *model.Addr { return &a }
	d := &model.Data{Bits: 8}
	v := func(f float64) model.Value { return model.Value{Conversion: model.Conversion{Factor: f}} }
	curve := func(key string, a model.Addr, n int, f float64) *model.Object {
		return &model.Object{Key: key, ID: key, Shape: "1d", Address: a, Rows: 1, Cols: n, Data: *d, Value: v(f)}
	}
	axis := func(a model.Addr, f float64) *model.Axis {
		return &model.Axis{Source: "image", Stored: "absolute", Address: addr(a), Data: d, Value: v(f)}
	}
	kf := func(key string, a model.Addr, x, y *model.Axis) *model.Object {
		return &model.Object{Key: key, ID: key, Shape: "2d", Address: a, Rows: 4, Cols: 4, Data: *d, X: x, Y: y}
	}
	hdr := axis(0x202, 1)
	hdr.Header = 2
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{
		curve("SNM", 0x100, 4, 0.0234375),
		curve("SNH", 0x200, 4, 1),
		curve("SN8", 0x600, 8, 1),
		curve("SNC", 0x700, 4, 40),
		kf("KF", 0x300, axis(0x100, 0.023438), hdr),
		kf("KF2", 0x400, axis(0x600, 1), axis(0x700, 0.75)),
		kf("KF3", 0x500, axis(0x300, 1), axis(0x800, 1)),
	}}
	b, err := Write(m, nil, "t")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `<embedinfo type="3" linkobjid="0x1">`) {
		t.Error("missing link to SNM")
	}
	if n := strings.Count(s, `type="3"`); n != 1 {
		t.Errorf("%d links, want 1", n)
	}
	r, _, err := Read(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if x := r.Objects[4].X; x == nil || x.Address == nil || *x.Address != 0x100 {
		t.Errorf("linked x axis read back as %+v", x)
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

// TestShownFormat checks the TunerPro display of a zero and of an identity
// conversion. Integer output type is used only when every value is a positive
// whole number. Min and max are written only when the model has both.
func TestShownFormat(t *testing.T) {
	img := make([]byte, 32)
	img[0], img[1] = 0, 5
	img[8], img[9], img[10], img[11] = 1, 2, 3, 4
	cell := func(id string, addr model.Addr) *model.Object {
		return &model.Object{
			Key: id, ID: id, Shape: "value", Address: addr, Rows: 1, Cols: 1,
			Data: model.Data{Bits: 8}, Value: model.Value{Conversion: model.Conversion{Factor: 1}},
		}
	}
	x := model.Addr(8)
	d := model.Data{Bits: 8, Signed: true}
	table := &model.Object{
		Key: "T", ID: "T", Shape: "1d", Address: 8, Rows: 1, Cols: 4,
		Data: d, Value: model.Value{Conversion: model.Conversion{Factor: 1}},
		X: &model.Axis{Source: "image", Stored: "absolute", Address: &x, Data: &d,
			Value: model.Value{Conversion: model.Conversion{Factor: 1}}},
		Y: &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}},
	}
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{
		cell("ZERO", 0), cell("POS", 1), table,
	}}
	root := parse(t, mustWrite(t, m, img))
	got := map[string]string{}
	var walk func(*node, string)
	walk = func(n *node, key string) {
		for _, c := range n.Kids {
			if c.Name == "title" {
				key = c.Text
			}
		}
		if n.Name == "XDFAXIS" {
			key += "/" + n.Attrs["id"]
		}
		if n.Name == "outputtype" || n.Name == "min" || n.Name == "max" {
			got[key+"/"+n.Name] = n.Text
		}
		if n.Name == "LABEL" && n.Attrs["index"] == "0" {
			got[key+"/label0"] = n.Attrs["value"]
		}
		for _, c := range n.Kids {
			walk(c, key)
		}
	}
	walk(root, "")
	if _, ok := got["ZERO/outputtype"]; ok {
		t.Errorf("zero cell output type %q", got["ZERO/outputtype"])
	}
	if got["POS/outputtype"] != "2" {
		t.Errorf("positive cell output type %q", got["POS/outputtype"])
	}
	if _, ok := got["T/z/min"]; ok {
		t.Errorf("invented cell range %q", got["T/z/min"])
	}
	if _, ok := got["T/x/min"]; ok {
		t.Errorf("invented axis range %q", got["T/x/min"])
	}
	if _, ok := got["T/y/outputtype"]; ok {
		t.Errorf("ordinal axis output type %q", got["T/y/outputtype"])
	}
	if got["T/y/label0"] != "0" {
		t.Errorf("ordinal label 0 %q", got["T/y/label0"])
	}
	if got["T/z/outputtype"] != "2" {
		t.Errorf("positive table output type %q", got["T/z/outputtype"])
	}
}

func TestSwappedLimits(t *testing.T) {
	lo, hi := 90.0, 10.0
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{{
		Key: "A", ID: "A", Shape: "1d", Address: 0, Rows: 1, Cols: 1,
		Data:  model.Data{Bits: 8},
		Value: model.Value{Min: &lo, Max: &hi, Conversion: model.Conversion{Factor: 1, Reciprocal: true}},
		X:     &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}},
	}}}
	b := mustWrite(t, m, make([]byte, 16))
	if !bytes.Contains(b, []byte("<min>10.000000</min>")) || !bytes.Contains(b, []byte("<max>90.000000</max>")) {
		t.Fatalf("range not ordered")
	}
}

// TestFloatTypeFlags: float cells get mmedtypeflags bit 0x10000 (and LE for
// wider cells). Confirmed in TunerPro 6.00.0149.00 (load and save).
func TestFloatTypeFlags(t *testing.T) {
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{{
		Key: "F", ID: "F", Shape: "value", Address: 0, Rows: 1, Cols: 1,
		Data:  model.Data{Bits: 32, Endian: "little", Float: true},
		Value: model.Value{Conversion: model.Conversion{Factor: 1}},
	}}}
	b := mustWrite(t, m, make([]byte, 4))
	if !bytes.Contains(b, []byte(`mmedtypeflags="0x10002"`)) {
		t.Fatalf("float LE flags missing: %s", b)
	}
	if !bytes.Contains(b, []byte(`version="1.80"`)) {
		t.Fatalf("want XDF 1.80: %s", b)
	}
	got, _, err := Read(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Objects[0].Data.Float || got.Objects[0].Data.Endian != "little" {
		t.Fatalf("read back %+v", got.Objects[0].Data)
	}
}

// TestFloatTableNonIdentityFails: TunerPro warns on non-X equations for float
// tables; the writer rejects that source conversion. Constants may use 2*X.
func TestFloatTableNonIdentityFails(t *testing.T) {
	img := make([]byte, 16)
	table := &model.Object{
		Key: "F4", ID: "F4", Shape: "1d", Address: 0, Rows: 1, Cols: 4,
		Data:  model.Data{Bits: 32, Endian: "little", Float: true},
		Value: model.Value{Conversion: model.Conversion{Factor: 2}},
		X:     &model.Axis{Source: "ordinal", Value: model.Value{Conversion: model.Conversion{Factor: 1}}},
	}
	_, err := Write(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{table}}, img, "t")
	if err == nil || !strings.Contains(err.Error(), "equation X") {
		t.Fatalf("want float equation error, got %v", err)
	}
	c := &model.Object{
		Key: "F1", ID: "F1", Shape: "value", Address: 0, Rows: 1, Cols: 1,
		Data:  model.Data{Bits: 32, Endian: "little", Float: true},
		Value: model.Value{Conversion: model.Conversion{Factor: 2}},
	}
	b, err := Write(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{c}}, img, "t")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`equation="2 * X"`)) {
		t.Fatalf("constant should keep 2*X: %s", b)
	}
}

func TestFloatBitsMustBe32(t *testing.T) {
	_, err := Write(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{{
		Key: "D1", ID: "D1", Shape: "value", Address: 0, Rows: 1, Cols: 1,
		Data:  model.Data{Bits: 64, Endian: "little", Float: true},
		Value: model.Value{Conversion: model.Conversion{Factor: 1}},
	}}}, make([]byte, 8), "t")
	if err == nil || !strings.Contains(err.Error(), "32 bits") {
		t.Fatalf("want 32-bit float error, got %v", err)
	}
}

// TestLiveSubtract writes an 8-bit subtract axis as the helper TunerPro
// 6.00.0149.00 displayed, and reads it back through the metadata file.
func TestLiveSubtract(t *testing.T) {
	addr := model.Addr(0x941C)
	ax := &model.Axis{
		Source: "image", Stored: "subtract", Address: &addr,
		Data: &model.Data{Bits: 8},
		Value: model.Value{
			Units: "grad C", Precision: 2,
			Conversion: model.Conversion{Factor: 0.75, Offset: -48},
		},
	}
	obj := func(id string, a model.Addr) *model.Object {
		x := *ax
		return &model.Object{
			Key: id, ID: id, Shape: "1d", Address: a, Rows: 1, Cols: 4,
			Data:  model.Data{Bits: 8},
			Value: model.Value{Precision: 3, Conversion: model.Conversion{Factor: 0.0625}},
			X:     &x,
		}
	}
	m := &model.Model{Schema: model.SchemaID, Objects: []*model.Object{obj("FHSA.0", 0x9420), obj("FHSA.1", 0x9430)}}
	b := mustWrite(t, m, nil)
	s := string(b)
	if strings.Count(s, `embedinfo type="3" linkobjid="0x3"`) != 2 {
		t.Errorf("links: %s", s)
	}
	if strings.Count(s, `equation="CELL(ROW()+1;0;FALSE) - 0.75 * X"`) != 3 {
		t.Errorf("cell rows: %s", s)
	}
	if strings.Count(s, `equation="(256 - X) * 0.75 - 48"`) != 1 {
		t.Errorf("last row: %s", s)
	}
	if strings.Count(s, livePrefix) != 1 {
		t.Errorf("helpers: %s", s)
	}
	meta, err := Meta(m, b, "xdfkit test")
	if err != nil {
		t.Fatal(err)
	}
	got, ws, err := Read(b, meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 0 {
		t.Errorf("warnings %q", ws)
	}
	if g, w := marshal(t, got), marshal(t, m); !bytes.Equal(g, w) {
		t.Errorf("round trip: %s", firstDiff(g, w))
	}
	v, _, err := Read(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Objects) != 2 {
		t.Errorf("view objects %d", len(v.Objects))
	}
	img := make([]byte, 0x9420)
	copy(img[0x941C:], []byte{27, 13, 12, 60})
	lb, err := WriteLabels(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{obj("FHSA.0", 0x9420)}}, img, "t")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(lb, []byte("CELL(ROW()")) {
		t.Errorf("static labels wrote a helper: %s", lb)
	}
	if !bytes.Contains(lb, []byte(`value="60.00"`)) || !bytes.Contains(lb, []byte(`value="80.25"`)) || !bytes.Contains(lb, []byte(`value="99.00"`)) {
		t.Errorf("labels: %s", lb)
	}
}

func mustWrite(t *testing.T, m *model.Model, img []byte) []byte {
	t.Helper()
	b, err := Write(m, img, "t")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
