package model

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/kp"
)

// ReadKP parses a KP file into the model. name is the original file name
// for the provenance (may be empty).
func ReadKP(data []byte, name string) (*Model, error) {
	f, err := kp.Parse(data)
	if err != nil {
		return nil, err
	}
	m := FromKP(f)
	sum := sha256.Sum256(data)
	m.Provenance = &Provenance{Format: "kp", Origin: "manual", File: name, SHA256: hex.EncodeToString(sum[:])}
	return m, nil
}

// FromKP converts a parsed KP file. The KP fields the model doesn't carry are
// dropped; KP takes them from a template.
func FromKP(f *kp.File) *Model {
	p := &f.Project
	m := &Model{Schema: SchemaID, Project: Project{p.Name, p.Version}}
	folders := map[int32]bool{}
	for _, fd := range p.Folders {
		m.Categories = append(m.Categories, &Category{ID: int(fd.ID), Name: fd.Name})
		folders[fd.ID] = true
	}
	for _, km := range p.Maps {
		m.Objects = append(m.Objects, objectFromKP(km, folders[km.FolderID]))
	}
	m.AssignKeys()
	m.CleanDisplay()
	return m
}

//go:embed empty.kp
var emptyKP []byte

//go:embed kp-defaults.json
var kpDefaultsJSON []byte

// defaultsKey is the kp-defaults.json block shared by every header pool.
// Each pool lists only the fields that differ; null drops a shared field.
const defaultsKey = "default"

// kpDefaults is that file expanded per header pool ("100", "200+300", "500")
// and record kind ("map", "axis", "slot" for undefined axes, "folder"): the
// commonest values of the KP fields the model doesn't carry. "500" comes from
// a local 597 pack when the file is regenerated.
var kpDefaults, kpShared = func() (pools map[string]map[string]map[string]any, base map[string]map[string]any) {
	raw, err := decodeDefaults(kpDefaultsJSON)
	if err != nil {
		panic(err)
	}
	return expandDefaults(raw)
}()

// decodeDefaults parses kp-defaults.json, turning "0x..." strings into numbers.
func decodeDefaults(b []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	raw, _ = unhex(raw).(map[string]any)
	return raw, nil
}

// defaultsFor is the expanded residue for a header pool. A pool that matches
// the shared default entirely is absent from the file and gets that default.
// An unknown header (empty key) gets nothing.
func defaultsFor(key string) map[string]map[string]any {
	if key == "" {
		return nil
	}
	if d, ok := kpDefaults[key]; ok {
		return d
	}
	return kpShared
}

// unhex turns the "0x..." strings that kp-defaults.json uses for round
// numbers into numbers. kp.Hex values are written without the prefix.
func unhex(v any) any {
	switch v := v.(type) {
	case string:
		if h, ok := strings.CutPrefix(v, "0x"); ok {
			if n, err := strconv.ParseInt(h, 16, 64); err == nil {
				return json.Number(strconv.FormatInt(n, 10))
			}
		}
	case []any:
		for i := range v {
			v[i] = unhex(v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = unhex(v[k])
		}
	}
	return v
}

// hexRound writes integers of at least 0x100 whose hex form is one digit
// followed by zeros, or all Fs, as "0x..." strings, so they read as the
// round numbers they are (kp-defaults.json).
func hexRound(v any) any {
	switch v := v.(type) {
	case json.Number:
		n, err := v.Int64()
		h := strings.TrimRight(strconv.FormatInt(n, 16), "0")
		if err == nil && n >= 0x100 && (len(h) == 1 || strings.Trim(h, "f") == "") {
			return fmt.Sprintf("0x%X", n)
		}
	case []any:
		for i := range v {
			v[i] = hexRound(v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = hexRound(v[k])
		}
	}
	return v
}

// collapseDefaults lifts every value shared by at least two header pools into
// the "default" block. Each pool keeps the merge patch against that block;
// null drops a shared field. full is pool, then record kind, then field.
func collapseDefaults(full map[string]map[string]map[string]any) map[string]any {
	var pools []string
	kinds := map[string]bool{}
	for pool, recs := range full {
		pools = append(pools, pool)
		for kind := range recs {
			kinds[kind] = true
		}
	}
	super := map[string]any{}
	for kind := range kinds {
		var objs []map[string]any
		for _, pool := range pools {
			if o := full[pool][kind]; o != nil {
				objs = append(objs, o)
			}
		}
		if m := majority(objs); len(m) > 0 {
			super[kind] = m
		}
	}
	out := map[string]any{}
	if len(super) > 0 {
		out[defaultsKey] = super
	}
	for _, pool := range pools {
		delta := map[string]any{}
		for kind, obj := range full[pool] {
			d, ok := canon.MergeDiff(super[kind], obj).(map[string]any)
			if !ok || len(d) == 0 {
				continue
			}
			delta[kind] = d
		}
		if len(delta) > 0 {
			out[pool] = delta
		}
	}
	return out
}

// majority keeps a key when one value is shared by at least two objects and
// more often than any other. Nested objects are split the same way, so a
// shared nested field lifts on its own.
func majority(objs []map[string]any) map[string]any {
	vals := map[string][]any{}
	for _, o := range objs {
		for k, v := range o {
			vals[k] = append(vals[k], v)
		}
	}
	out := map[string]any{}
	for k, vs := range vals {
		if nested, ok := asMaps(vs); ok {
			if m := majority(nested); len(m) > 0 {
				out[k] = m
			}
			continue
		}
		if v, ok := mode(vs); ok {
			out[k] = v
		}
	}
	return out
}

// mode is the value shared most often, when that is at least twice and unique.
func mode(vs []any) (any, bool) {
	n := map[string]int{}
	val := map[string]any{}
	for _, v := range vs {
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		raw := string(b)
		n[raw]++
		val[raw] = v
	}
	var best string
	bestN, next := 0, 0
	for raw, c := range n {
		if c > bestN {
			next, best, bestN = bestN, raw, c
		} else if c > next {
			next = c
		}
	}
	if bestN < 2 || bestN == next {
		return nil, false
	}
	return val[best], true
}

func asMaps(vs []any) ([]map[string]any, bool) {
	out := make([]map[string]any, len(vs))
	for i, v := range vs {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		out[i] = m
	}
	return out, true
}

// expandDefaults is the inverse of collapseDefaults. base is the shared block
// alone; pools have that block with each pool's patch applied.
func expandDefaults(collapsed map[string]any) (pools map[string]map[string]map[string]any, base map[string]map[string]any) {
	baseAny, _ := collapsed[defaultsKey].(map[string]any)
	base = map[string]map[string]any{}
	for kind, obj := range baseAny {
		m, _ := obj.(map[string]any)
		base[kind], _ = canon.MergePatch(m, map[string]any{}).(map[string]any)
	}
	pools = map[string]map[string]map[string]any{}
	for pool, kindsAny := range collapsed {
		if pool == defaultsKey {
			continue
		}
		kinds, _ := kindsAny.(map[string]any)
		pools[pool] = applyDefault(base, kinds)
	}
	return pools, base
}

func applyDefault(base map[string]map[string]any, delta map[string]any) map[string]map[string]any {
	seen := map[string]bool{}
	for kind := range base {
		seen[kind] = true
	}
	for kind := range delta {
		seen[kind] = true
	}
	out := map[string]map[string]any{}
	for kind := range seen {
		patch, ok := delta[kind]
		if !ok {
			patch = map[string]any{}
		}
		if m, ok := canon.MergePatch(base[kind], patch).(map[string]any); ok && len(m) > 0 {
			out[kind] = m
		}
	}
	return out
}

// KP converts the model to a KP file. Fields the model doesn't carry come from
// the template's map with the same key, or folder with the same id, and
// otherwise from kpDefaults; the file header and trailing data come from the
// template. A nil template means an empty WinOLS 2.24 project (header 292). With the
// file the model was read from as template, KP reproduces it.
func (m *Model) KP(template *kp.File) (*kp.File, error) {
	if err := m.Check(); err != nil {
		return nil, err
	}
	t := template
	if t == nil {
		var err error
		if t, err = kp.Parse(emptyKP); err != nil {
			return nil, err
		}
		if m.Provenance != nil && m.Provenance.File != "" {
			t.Filename = m.Provenance.File
		}
	}
	d := defaultsFor(t.DefaultsKey())
	f := *t
	f.Project.Name, f.Project.Version = m.Project.Name, m.Project.Version
	f.Project.Maps, f.Project.Folders = nil, nil

	tm := FromKP(t)
	folders := map[int]map[string]any{}
	for i, c := range tm.Categories {
		r, err := residue(folderDisplay(t.Project.Folders[i]), c.kpFolder())
		if err != nil {
			return nil, err
		}
		folders[c.ID] = r
	}
	maps := map[string][]map[string]any{}
	for i, o := range tm.Objects {
		r, err := residue(mapDisplay(t.Project.Maps[i]), o.kpMap(i))
		if err != nil {
			return nil, err
		}
		maps[o.Key] = append(maps[o.Key], r)
	}

	for _, c := range m.Categories {
		r, ok := folders[c.ID]
		if !ok {
			r = d["folder"]
		}
		fd := new(kp.Folder)
		if err := restore(fd, c.kpFolder(), r); err != nil {
			return nil, fmt.Errorf("category %d: %w", c.ID, err)
		}
		f.Project.Folders = append(f.Project.Folders, fd)
	}
	for i, o := range m.Objects {
		var r map[string]any
		fromTemplate := false
		if q := maps[o.Key]; len(q) > 0 {
			r = q[0]
			maps[o.Key] = q[1:]
			fromTemplate = true
		} else {
			r = map[string]any{}
			for k, v := range d["map"] {
				r[k] = v
			}
			for k, x := range map[string]*Axis{"x": o.X, "y": o.Y} {
				if a := d[axisKind(x)]; a != nil {
					r[k] = a
				}
			}
		}
		km := new(kp.Map)
		if err := restore(km, o.kpMap(i), r); err != nil {
			return nil, fmt.Errorf("object %s: %w", o.Key, err)
		}
		if !fromTemplate {
			kpValueRange(km, o)
		}
		km.Index, km.Key = i, o.Key
		f.Project.Maps = append(f.Project.Maps, km)
	}
	return &f, nil
}

func axisKind(x *Axis) string {
	if x == nil {
		return "slot"
	}
	return "axis"
}

func (c *Category) kpFolder() *kp.Folder { return &kp.Folder{ID: int32(c.ID), Name: c.Name} }

// folderDisplay and mapDisplay are the template records with display text
// cleaned the same way as the model, so original spacing is not residue.
func folderDisplay(fd *kp.Folder) *kp.Folder {
	c := *fd
	c.Name = CleanText(c.Name)
	return &c
}

func mapDisplay(m *kp.Map) *kp.Map {
	c := *m
	c.Name = CleanText(c.Name)
	c.Comment = strings.TrimSpace(c.Comment)
	c.Value = cleanKPValue(c.Value)
	if c.X != nil {
		x := *c.X
		x.Value = cleanKPValue(x.Value)
		c.X = &x
	}
	if c.Y != nil {
		y := *c.Y
		y.Value = cleanKPValue(y.Value)
		c.Y = &y
	}
	return &c
}

func cleanKPValue(v kp.Value) kp.Value {
	v.Description = CleanText(v.Description)
	v.Units = CleanText(v.Units)
	return v
}

var shapes = map[kp.Org]string{kp.OrgSingle: "value", kp.OrgOneD: "1d", kp.OrgTwoD: "2d", kp.OrgTwoDInv: "2d"}

func objectFromKP(km *kp.Map, inFolder bool) *Object {
	o := &Object{
		ID:          km.ID,
		Description: km.Name,
		Comment:     km.Comment,
		Shape:       shapes[km.Type],
		Inverse:     km.Type == kp.OrgTwoDInv,
		Address:     Addr(km.Start),
		Rows:        int(km.Rows),
		Cols:        int(km.Cols),
		Data:        dataFromKP(km.DataOrg, km.Signed),
		Value:       valueFromKP(km.Value, int(km.Precision), km.Reciprocal),
		View:        View{Radix: int(km.Radix), Difference: km.Difference, Percent: km.Percent, Original: km.Original != 0},
		X:           axisFromKP(km.X, axisCount(km, true)),
		Y:           axisFromKP(km.Y, axisCount(km, false)),
	}
	if km.Marked != 0 {
		o.VisLevel = 1
	}
	if inFolder {
		o.Categories = []int{int(km.FolderID)}
	}
	switch {
	case o.Shape != "":
	case o.Rows > 1:
		o.Shape = "2d"
	case o.Cols > 1:
		o.Shape = "1d"
	default:
		o.Shape = "value"
	}
	valueRangeFromKP(km, o)
	return o
}

// axisCount is the free-editable Values length: StoredCols/StoredRows when set
// (swapped on 2d-inverse), else Cols/Rows.
func axisCount(km *kp.Map, x bool) int {
	if x {
		if km.StoredCols > 0 {
			return int(km.StoredCols)
		}
		return int(km.Cols)
	}
	if km.StoredRows > 0 {
		return int(km.StoredRows)
	}
	return int(km.Rows)
}

func (o *Object) modelAxisCount(x bool) int {
	c, r := o.Cols, o.Rows
	if o.Inverse {
		c, r = r, c
	}
	if x {
		return c
	}
	return r
}

func (o *Object) kpMap(i int) *kp.Map {
	org := map[string]kp.Org{"value": kp.OrgSingle, "1d": kp.OrgOneD, "2d": kp.OrgTwoD}[o.Shape]
	if o.Inverse {
		org = kp.OrgTwoDInv
	}
	t := kpDataOrg(o.Data)
	km := &kp.Map{
		Index:      i,
		Key:        o.Key,
		Comment:    o.Comment,
		Name:       o.Description,
		Type:       org,
		DataOrg:    t,
		Width:      int32(t.Width()),
		Radix:      kpRadix(o.View.Radix),
		ID:         o.ID,
		Reciprocal: o.Value.Conversion.Reciprocal,
		Signed:     o.Data.Signed,
		Difference: o.View.Difference,
		Percent:    o.View.Percent,
		Cols:       int32(o.Cols),
		Rows:       int32(o.Rows),
		Precision:  int32(o.Value.Precision),
		Value:      kpValue(o.Value),
		Start:      uint32(o.Address),
		End:        uint32(o.Address) + uint32(o.Rows*o.Cols*t.Width()),
		Addr2:      uint32(o.Address),
		X:          o.X.kpAxis(o.modelAxisCount(true)),
		Y:          o.Y.kpAxis(o.modelAxisCount(false)),
		StoredCols: int32(o.Cols),
		StoredRows: int32(o.Rows),
		Term2:      kp.Hex{1, 1, 1},
	}
	if o.Inverse {
		km.StoredCols, km.StoredRows = km.StoredRows, km.StoredCols
	}
	if len(o.Categories) > 0 {
		km.FolderID = int32(o.Categories[0])
	}
	if o.VisLevel == 1 {
		km.Marked = 1
	}
	if o.View.Original {
		km.Original = 1
	}
	return km
}

func dataFromKP(t kp.DataOrg, signed bool) Data {
	d := Data{Bits: 8 * t.Width(), Signed: signed, Float: t.Float()}
	if d.Bits == 0 {
		d.Bits = 8
	}
	if d.Bits > 8 {
		d.Endian = map[bool]string{true: "little", false: "big"}[t.LE()]
	}
	return d
}

func kpDataOrg(d Data) kp.DataOrg {
	le := kp.DataOrg(0)
	if d.Endian == "little" {
		le = 1
	}
	switch {
	case d.Bits == 16:
		return 2 + le
	case d.Bits == 32 && d.Float:
		return 6 + le
	case d.Bits == 32:
		return 4 + le
	}
	return 1
}

func valueFromKP(v kp.Value, precision int, reciprocal bool) Value {
	return Value{Description: v.Description, Units: v.Units, Precision: precision, Conversion: Conversion{Factor: v.Factor, Offset: v.Offset, Reciprocal: reciprocal}}
}

func kpValue(v Value) kp.Value {
	return kp.Value{Description: v.Description, Units: v.Units, Factor: v.Conversion.Factor, Offset: v.Conversion.Offset}
}

// rawRangeDefaults is the WinOLS default Value range pair for the map width.
func rawRangeDefaults(bits int) (lo, hi int64, ok bool) {
	switch bits {
	case 8:
		return 0, 255, true
	case 16:
		return 0, 65535, true
	case 32:
		return 0, 4294967295, true
	}
	return 0, 0, false
}

// valueRangeFromKP sets model min/max from the KP raw range pair for the map
// width, converted to display units. The default raw pair is omitted.
func valueRangeFromKP(km *kp.Map, o *Object) {
	if o.Data.Float {
		return
	}
	defLo, defHi, ok := rawRangeDefaults(o.Data.Bits)
	if !ok {
		return
	}
	s := min(o.Data.Bits/16, 2)
	lo, hi := km.Range[s][0], km.Range[s][1]
	if lo == defLo && hi == defHi {
		return
	}
	dlo, dhi := o.Value.Conversion.Apply(float64(lo)), o.Value.Conversion.Apply(float64(hi))
	if math.IsNaN(dlo+dhi) || math.IsInf(dlo, 0) || math.IsInf(dhi, 0) {
		return
	}
	if dlo > dhi {
		dlo, dhi = dhi, dlo
	}
	o.Value.Min, o.Value.Max = &dlo, &dhi
}

// kpValueRange sets the KP raw value-range pair for the map's width when the
// map did not come from a template. Model min/max (display) are converted to
// raw. Otherwise the default raw pair for the width is written. The field
// cannot be omitted.
func kpValueRange(km *kp.Map, o *Object) {
	defLo, defHi, ok := rawRangeDefaults(o.Data.Bits)
	if !ok || o.Data.Float {
		return
	}
	s := min(o.Data.Bits/16, 2)
	km.Range[s][0], km.Range[s][1] = defLo, defHi
	if o.Value.Min == nil || o.Value.Max == nil {
		return
	}
	a := o.Value.Conversion.Unapply(*o.Value.Min)
	b := o.Value.Conversion.Unapply(*o.Value.Max)
	if math.IsNaN(a+b) || math.IsInf(a, 0) || math.IsInf(b, 0) {
		return
	}
	lo, hi := math.Round(a), math.Round(b)
	if lo > hi {
		lo, hi = hi, lo
	}
	km.Range[s][0], km.Range[s][1] = int64(lo), int64(hi)
}

// Limits is the conversion of the raw endpoints, ordered. Identity uses the
// unsigned span. A reciprocal conversion maps the low raw value to the high
// physical value. Float and a width of 0 have none. The XDF reader leaves out
// a min and max that are only this span. The KP writer uses the default raw
// pair when the model has no min and max.
func Limits(c Conversion, d Data) (lo, hi float64, ok bool) {
	if d.Float || d.Bits <= 0 || d.Bits > 32 {
		return 0, 0, false
	}
	rlo, rhi := int64(0), int64(1)<<d.Bits-1
	ident := c.Factor == 1 && c.Offset == 0 && !c.Reciprocal
	switch {
	case d.Signed && !ident:
		rlo, rhi = -(1 << (d.Bits - 1)), 1<<(d.Bits-1)-1
	case c.Reciprocal:
		rlo = 1
	}
	a, b := c.Apply(float64(rlo)), c.Apply(float64(rhi))
	return math.Min(a, b), math.Max(a, b), true
}

// Apply is factor*raw+offset, or factor/raw+offset when reciprocal. The
// product is a separate step so it does not fuse, matching mapdump's Java.
func (c Conversion) Apply(raw float64) float64 {
	if c.Reciprocal {
		return c.Factor/raw + c.Offset
	}
	return float64(raw*c.Factor) + c.Offset
}

// Unapply is the inverse of Apply: raw from a physical value.
func (c Conversion) Unapply(phys float64) float64 {
	if c.Reciprocal {
		d := phys - c.Offset
		if d == 0 {
			return math.NaN()
		}
		return c.Factor / d
	}
	if c.Factor == 0 {
		return math.NaN()
	}
	return (phys - c.Offset) / c.Factor
}

// Axis sources and storage by KP datasource (docs/kp-format.md).
var axisSources = []struct{ source, stored string }{
	kp.DSOrdinal:         {"ordinal", ""},
	kp.DSEeprom:          {"image", "absolute"},
	kp.DSEepromAdd:       {"image", "add"},
	kp.DSEepromSubtract:  {"image", "subtract"},
	kp.DSFreeEditable:    {"editable", ""},
	kp.DSEepromBackwards: {"image", "backwards"},
}

const noSignature = -1

func axisFromKP(a *kp.Axis, n int) *Axis {
	if a == nil || !a.Defined {
		return nil
	}
	x := &Axis{
		Source:    "unknown",
		Value:     valueFromKP(a.Value, int(a.Precision), a.Reciprocal),
		View:      View{Radix: int(a.Radix)},
		Backwards: a.Backwards,
		Header:    int(a.DataHeader),
	}
	if a.SignatureByte != noSignature {
		s := int(a.SignatureByte)
		x.Signature = &s
	}
	if ds := int(a.DataSource); ds >= 0 && ds < len(axisSources) {
		x.Source, x.Stored = axisSources[ds].source, axisSources[ds].stored
	}
	d := dataFromKP(a.DataOrg, a.Signed)
	switch x.Source {
	case "image":
		addr := Addr(a.Addr)
		x.Address, x.Data = &addr, &d
	case "editable":
		x.Data = &d
		x.Values = unpackAxisValues(a.Values, d, x.Value.Conversion, n)
	}
	return x
}

// kpAxis rebuilds the axis record from the model. Width is derived from the
// type, as WinOLS stores it (docs/kp-format.md). n is the axis point count
// for packing editable Values.
func (x *Axis) kpAxis(n int) *kp.Axis {
	a := &kp.Axis{Values: kp.Hex{}}
	if x == nil {
		a.Value = kp.Value{Description: "-", Units: "-"} // as WinOLS fills unused slots
		return a
	}
	a.Defined = true
	a.Backwards = x.Backwards
	a.DataHeader = int32(x.Header)
	a.SignatureByte = noSignature
	if x.Signature != nil {
		a.SignatureByte = int32(*x.Signature)
	}
	a.Value = kpValue(x.Value)
	a.Reciprocal = x.Value.Conversion.Reciprocal
	a.Precision = int32(x.Value.Precision)
	a.Radix = kpRadix(x.View.Radix)
	for ds, s := range axisSources {
		if s.source == x.Source && s.stored == x.Stored {
			a.DataSource = kp.DataSource(ds)
		}
	}
	if x.Address != nil {
		a.Addr = uint32(*x.Address)
	}
	if x.Data != nil {
		a.DataOrg, a.Signed = kpDataOrg(*x.Data), x.Data.Signed
	} else {
		// No storage. DataOrg 0 has no width. An 8-bit ordinal is the unused-slot width.
		a.DataOrg = 1
	}
	a.Width = int32(a.DataOrg.Width())
	if x.Source == "editable" && n > 0 && x.Data != nil && !x.Data.Float {
		a.Values = packAxisValues(x.Values, *x.Data, x.Value.Conversion, n)
	}
	return a
}

// unpackAxisValues reads consecutive axis-type elements from h (padded to
// 4*n), converts them to display units, and drops a trailing all-zero list.
func unpackAxisValues(h kp.Hex, d Data, c Conversion, n int) []float64 {
	w := d.Bits / 8
	if n < 1 || w < 1 || d.Float || len(h) < n*w {
		return nil
	}
	var order binary.ByteOrder = binary.LittleEndian
	if d.Endian == "big" {
		order = binary.BigEndian
	}
	out := make([]float64, n)
	nonzero := false
	for i := range n {
		raw := axisRaw(h[i*w:], order, w, d.Signed)
		if raw != 0 {
			nonzero = true
		}
		out[i] = c.Apply(float64(raw))
	}
	if !nonzero {
		return nil
	}
	for len(out) > 0 && out[len(out)-1] == 0 {
		out = out[:len(out)-1]
	}
	return out
}

// packAxisValues writes display values as consecutive axis-type elements,
// zero-padded to 4*n bytes.
func packAxisValues(vals []float64, d Data, c Conversion, n int) kp.Hex {
	w := d.Bits / 8
	if n < 1 || w < 1 || d.Float {
		return kp.Hex{}
	}
	var order binary.ByteOrder = binary.LittleEndian
	if d.Endian == "big" {
		order = binary.BigEndian
	}
	out := make(kp.Hex, 4*n)
	for i := 0; i < n && i < len(vals); i++ {
		raw := int64(math.Round(c.Unapply(vals[i])))
		putAxisRaw(out[i*w:], order, w, d.Signed, raw)
	}
	return out
}

func axisRaw(b []byte, order binary.ByteOrder, w int, signed bool) int64 {
	var u uint64
	switch w {
	case 1:
		u = uint64(b[0])
	case 2:
		u = uint64(order.Uint16(b))
	case 4:
		u = uint64(order.Uint32(b))
	default:
		return 0
	}
	if signed {
		shift := 64 - 8*w
		return int64(u) << shift >> shift
	}
	return int64(u)
}

func putAxisRaw(b []byte, order binary.ByteOrder, w int, signed bool, raw int64) {
	var u uint64
	if signed {
		u = uint64(raw)
	} else if raw < 0 {
		u = 0
	} else {
		u = uint64(raw)
	}
	switch w {
	case 1:
		b[0] = byte(u)
	case 2:
		order.PutUint16(b, uint16(u))
	case 4:
		order.PutUint32(b, uint32(u))
	}
}

// kpBase is the KP display base. 0 is omitted in the model; WinOLS crashes
// when a map with base 0 is opened. 10, 16 and 2 are written as given.
func kpRadix(b int) int32 {
	if b == 0 {
		return 10
	}
	return int32(b)
}
