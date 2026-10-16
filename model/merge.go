package model

import (
	"fmt"
	"slices"
	"strings"

	"go.nyet.org/xdfkit/canon"
)

// MergeOptions selects how Merge combines two definitions. Description is
// keep, fill or replace. Comment is keep, fill, append or replace. Empty
// selects the mode default: keep unless Update (fill and append) or Overwrite
// (replace).
type MergeOptions struct {
	Smart       bool
	Update      bool
	Overwrite   bool
	Description string
	Comment     string
}

// Action is one merge decision. Key is the object's name. Field names the
// value the action is about, when there is one.
type Action struct {
	Action string `json:"action"`
	Key    string `json:"key,omitempty"`
	Field  string `json:"field,omitempty"`
}

// Merge folds incoming into a copy of base. A blank key and a blank id match
// nothing. See docs/design.md.
func Merge(base, incoming *Model, opt MergeOptions) (*Model, []Action, error) {
	desc, comm, err := policies(opt)
	if err != nil {
		return nil, nil, err
	}
	if base == nil {
		return nil, nil, fmt.Errorf("merge: no base definition")
	}
	out, err := cloneModel(base)
	if err != nil {
		return nil, nil, err
	}
	var inc *Model
	if incoming != nil {
		if inc, err = cloneModel(incoming); err != nil {
			return nil, nil, err
		}
	}
	var actions []Action
	if opt.Smart {
		out.Objects, actions = fold(out.Objects, actions)
		if inc != nil {
			inc.Objects, actions = fold(inc.Objects, actions)
		}
	}
	if inc == nil {
		return out, actions, out.Check()
	}
	byName := map[string][]*Object{}
	for _, o := range out.Objects {
		if n := nameOf(o); n != "" {
			byName[n] = append(byName[n], o)
		}
	}
	inByName := map[string][]*Object{}
	for _, o := range inc.Objects {
		if n := nameOf(o); n != "" {
			inByName[n] = append(inByName[n], o)
		}
	}
	consumed := map[*Object]bool{}
	seenName := map[string]bool{}
	for _, o := range inc.Objects {
		n := nameOf(o)
		if n == "" || seenName[n] {
			continue
		}
		bases := byName[n]
		if len(bases) == 0 {
			continue
		}
		seenName[n] = true
		ins := inByName[n]
		for _, in := range ins {
			consumed[in] = true
		}
		if len(bases) != 1 || len(ins) != 1 {
			actions = append(actions, Action{Action: "ambiguous", Key: n})
			if d, c, ok := sameText(ins); ok {
				for _, b := range bases {
					actions = applyText(actions, b, d, c, desc, comm)
				}
			}
			continue
		}
		b, in := bases[0], ins[0]
		switch {
		case opt.Overwrite:
			if err := replaceObject(out, inc, b, in); err != nil {
				return nil, nil, err
			}
			actions = append(actions, Action{Action: "overwritten", Key: n})
			actions = applyText(actions, b, in.Description, in.Comment, desc, comm)
		case opt.Update:
			before := len(actions)
			for _, f := range copyDetected(b, in) {
				actions = append(actions, Action{Action: "corrected", Key: n, Field: f})
			}
			for _, f := range defaultDiff(b, in) {
				actions = append(actions, Action{Action: "default", Key: n, Field: f})
			}
			if u := unitsDiff(b, in); u != "" {
				actions = append(actions, Action{Action: u, Key: n, Field: "units"})
			}
			if catsDiffer(out, b, inc, in) {
				actions = append(actions, Action{Action: "categories", Key: n})
			}
			actions = applyText(actions, b, in.Description, in.Comment, desc, comm)
			if len(actions) == before {
				actions = append(actions, Action{Action: "kept", Key: n})
			}
		default:
			before := len(actions)
			actions = applyText(actions, b, in.Description, in.Comment, desc, comm)
			if len(actions) == before {
				actions = append(actions, Action{Action: "kept", Key: n})
			}
		}
	}
	add := func(in *Object) error {
		c, err := adopt(out, inc, in)
		if err != nil {
			return err
		}
		out.Objects = append(out.Objects, c)
		return nil
	}
	for _, in := range inc.Objects {
		if consumed[in] {
			continue
		}
		n := nameOf(in)
		if i := overlapIndex(out.Objects, in); i >= 0 {
			if err := add(in); err != nil {
				return nil, nil, err
			}
			actions = append(actions, Action{Action: "overlap", Key: n})
			actions = append(actions, Action{Action: "added", Key: n})
			actions = reportNewDefaults(actions, n, in)
			continue
		}
		same := sameSpan(out.Objects, in)
		switch {
		case len(same) == 0:
			if err := add(in); err != nil {
				return nil, nil, err
			}
			actions = append(actions, Action{Action: "added", Key: n})
			actions = reportNewDefaults(actions, n, in)
		case !sameLayout(same[0], in):
			if err := add(in); err != nil {
				return nil, nil, err
			}
			actions = append(actions, Action{Action: "kept", Key: n, Field: "span"})
		case opt.Smart:
			c, err := adopt(out, inc, in)
			if err != nil {
				return nil, nil, err
			}
			actions = placeSmart(actions, same[0], c, n)
		case opt.Overwrite && len(same) == 1:
			if err := replaceObject(out, inc, same[0], in); err != nil {
				return nil, nil, err
			}
			actions = append(actions, Action{Action: "overwritten", Key: n})
		default:
			actions = append(actions, Action{Action: "skipped", Key: n, Field: "span"})
		}
	}
	missed := map[string]bool{}
	for _, o := range out.Objects {
		n := nameOf(o)
		if n == "" || len(inByName[n]) > 0 || missed[n] {
			continue
		}
		missed[n] = true
		actions = append(actions, Action{Action: "missing", Key: n})
	}
	return out, actions, out.Check()
}

func policies(opt MergeOptions) (desc, comm string, err error) {
	desc, comm = "keep", "keep"
	switch {
	case opt.Overwrite:
		desc, comm = "replace", "replace"
	case opt.Update:
		desc, comm = "fill", "append"
	}
	if opt.Description != "" {
		desc = opt.Description
	}
	if opt.Comment != "" {
		comm = opt.Comment
	}
	if desc != "keep" && desc != "fill" && desc != "replace" {
		return "", "", fmt.Errorf("description policy %q", desc)
	}
	if comm != "keep" && comm != "fill" && comm != "append" && comm != "replace" {
		return "", "", fmt.Errorf("comment policy %q", comm)
	}
	return desc, comm, nil
}

func cloneModel(m *Model) (*Model, error) {
	b, err := canon.Marshal(m)
	if err != nil {
		return nil, err
	}
	out := new(Model)
	if err := canon.Unmarshal(b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func nameOf(o *Object) string {
	if o == nil {
		return ""
	}
	if o.Key != "" {
		return o.Key
	}
	return strings.TrimSpace(o.ID)
}

func sameText(os []*Object) (desc, comm string, ok bool) {
	desc, comm = os[0].Description, os[0].Comment
	for _, o := range os[1:] {
		if o.Description != desc || o.Comment != comm {
			return "", "", false
		}
	}
	return desc, comm, true
}

func applyText(actions []Action, o *Object, desc, comm, dp, cp string) []Action {
	next, report := textPolicy(o.Description, desc, dp, false)
	if report {
		actions = append(actions, Action{Action: "description", Key: nameOf(o)})
	}
	o.Description = next
	next, report = textPolicy(o.Comment, comm, cp, true)
	if report {
		actions = append(actions, Action{Action: "comment", Key: nameOf(o)})
	}
	o.Comment = next
	return actions
}

func textPolicy(base, in, policy string, comment bool) (string, bool) {
	if in == "" {
		return base, false
	}
	switch policy {
	case "fill":
		if base == "" {
			return in, true
		}
		return base, false
	case "replace":
		return in, base != in
	case "append":
		if !comment {
			return base, false
		}
		next := appendLines(base, in)
		return next, next != base
	default:
		return base, base != in
	}
}

func appendLines(base, in string) string {
	have := map[string]bool{}
	for _, l := range strings.Split(base, "\n") {
		have[l] = true
	}
	for _, l := range strings.Split(in, "\n") {
		if l == "" || have[l] {
			continue
		}
		if base != "" {
			base += "\n"
		}
		base += l
		have[l] = true
	}
	return base
}

func replaceObject(dst, src *Model, b, in *Object) error {
	c, err := cloneObject(in)
	if err != nil {
		return err
	}
	key, id := b.Key, b.ID
	desc, comm := b.Description, b.Comment
	cats := remapCats(dst, src, in.Categories)
	*b = *c
	b.Key, b.ID = key, id
	b.Description, b.Comment = desc, comm
	b.Categories = cats
	return nil
}

func cloneObject(o *Object) (*Object, error) {
	b, err := canon.Marshal(o)
	if err != nil {
		return nil, err
	}
	out := new(Object)
	if err := canon.Unmarshal(b, out); err != nil {
		return nil, err
	}
	return out, nil
}

func copyDetected(dst, src *Object) []string {
	var c []string
	if dst.Address != src.Address {
		dst.Address = src.Address
		c = append(c, "address")
	}
	if dst.Shape != src.Shape {
		dst.Shape = src.Shape
		c = append(c, "shape")
	}
	if dst.Rows != src.Rows {
		dst.Rows = src.Rows
		c = append(c, "rows")
	}
	if dst.Cols != src.Cols {
		dst.Cols = src.Cols
		c = append(c, "cols")
	}
	if dst.Data.Bits != src.Data.Bits {
		dst.Data.Bits = src.Data.Bits
		c = append(c, "bits")
	}
	if dst.Data.Signed != src.Data.Signed {
		dst.Data.Signed = src.Data.Signed
		c = append(c, "signed")
	}
	if copyAxisDetected(&dst.X, src.X) {
		c = append(c, "x")
	}
	if copyAxisDetected(&dst.Y, src.Y) {
		c = append(c, "y")
	}
	return c
}

func copyAxisDetected(dst **Axis, src *Axis) bool {
	if src == nil || src.Address == nil {
		return false
	}
	if *dst == nil {
		a := &Axis{Source: src.Source, Address: addrCopy(src.Address)}
		if src.Data != nil {
			a.Data = &Data{Bits: src.Data.Bits}
		}
		*dst = a
		return true
	}
	a := *dst
	changed := false
	if src.Source != "" && a.Source != src.Source {
		a.Source = src.Source
		changed = true
	}
	if a.Address == nil || *a.Address != *src.Address {
		a.Address = addrCopy(src.Address)
		changed = true
	}
	if src.Data != nil {
		if a.Data == nil {
			a.Data = &Data{Bits: src.Data.Bits}
			changed = true
		} else if a.Data.Bits != src.Data.Bits {
			a.Data.Bits = src.Data.Bits
			changed = true
		}
	}
	return changed
}

func addrCopy(a *Addr) *Addr {
	if a == nil {
		return nil
	}
	v := *a
	return &v
}

func defaultDiff(base, in *Object) []string {
	var s []string
	if base.Data.Endian != in.Data.Endian {
		s = append(s, "endian")
	}
	if base.Data.Float != in.Data.Float {
		s = append(s, "float")
	}
	if base.Value.Conversion != in.Value.Conversion || base.Value.Description != in.Value.Description {
		s = append(s, "conversion")
	}
	if base.Value.Precision != in.Value.Precision {
		s = append(s, "precision")
	}
	if in.Value.Units == "" && base.Value.Units != "" {
		s = append(s, "units")
	}
	if base.View != in.View {
		s = append(s, "view")
	}
	if !sameLimits(base.Value, in.Value) {
		s = append(s, "limits")
	}
	if base.VisLevel != in.VisLevel {
		s = append(s, "vislevel")
	}
	if base.Inverse != in.Inverse {
		s = append(s, "inverse")
	}
	s = append(s, axisDefaultDiff("x", base.X, in.X)...)
	s = append(s, axisDefaultDiff("y", base.Y, in.Y)...)
	return s
}

func axisDefaultDiff(id string, base, in *Axis) []string {
	if base == nil || in == nil {
		return nil
	}
	var s []string
	if base.Stored != in.Stored {
		s = append(s, id+".stored")
	}
	if base.Data != nil && in.Data != nil && base.Data.Signed != in.Data.Signed {
		s = append(s, id+".signed")
	}
	if base.Value.Conversion != in.Value.Conversion {
		s = append(s, id+".conversion")
	}
	if base.View != in.View {
		s = append(s, id+".view")
	}
	if base.Mirror != in.Mirror {
		s = append(s, id+".mirror")
	}
	if base.Header != in.Header {
		s = append(s, id+".header")
	}
	if (base.Signature == nil) != (in.Signature == nil) || base.Signature != nil && *base.Signature != *in.Signature {
		s = append(s, id+".signature")
	}
	return s
}

func unitsDiff(base, in *Object) string {
	if in.Value.Units == "" || in.Value.Units == base.Value.Units {
		return ""
	}
	return "units"
}

func sameLimits(a, b Value) bool {
	return sameF(a.Min, b.Min) && sameF(a.Max, b.Max)
}

func sameF(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func catsDiffer(base *Model, b *Object, inc *Model, in *Object) bool {
	return !slices.Equal(catNames(base, b.Categories), catNames(inc, in.Categories))
}

func catNames(m *Model, ids []int) []string {
	var s []string
	for _, id := range ids {
		for _, c := range m.Categories {
			if c.ID == id {
				s = append(s, c.Name)
			}
		}
	}
	slices.Sort(s)
	return s
}

func reportNewDefaults(actions []Action, key string, o *Object) []Action {
	c := o.Value.Conversion
	if c.Factor == 1 && c.Offset == 0 && !c.Reciprocal {
		actions = append(actions, Action{Action: "default", Key: key, Field: "conversion"})
	}
	if o.Value.Precision == 0 {
		actions = append(actions, Action{Action: "default", Key: key, Field: "precision"})
	}
	if o.View == (View{}) {
		actions = append(actions, Action{Action: "default", Key: key, Field: "view"})
	}
	if o.Value.Min == nil && o.Value.Max == nil {
		actions = append(actions, Action{Action: "default", Key: key, Field: "limits"})
	}
	if o.Value.Units == "" {
		actions = append(actions, Action{Action: "default", Key: key, Field: "units"})
	}
	return actions
}

func adopt(dst, src *Model, o *Object) (*Object, error) {
	c, err := cloneObject(o)
	if err != nil {
		return nil, err
	}
	c.Categories = remapCats(dst, src, o.Categories)
	return c, nil
}

func remapCats(dst, src *Model, ids []int) []int {
	if src == nil {
		src = dst
	}
	var out []int
	for _, id := range ids {
		name := ""
		for _, c := range src.Categories {
			if c.ID == id {
				name = c.Name
			}
		}
		if name == "" {
			continue
		}
		out = append(out, dst.category(name))
	}
	return out
}

func (m *Model) category(name string) int {
	for _, c := range m.Categories {
		if c.Name == name {
			return c.ID
		}
	}
	id := 0
	for _, c := range m.Categories {
		if c.ID >= id {
			id = c.ID + 1
		}
	}
	m.Categories = append(m.Categories, &Category{ID: id, Name: name})
	return id
}

type cellSpan struct {
	addr       Addr
	rows, cols int
	bits       int
}

func spanOf(o *Object) cellSpan {
	bits := o.Data.Bits
	if bits == 0 {
		bits = 8
	}
	rows, cols := o.Rows, o.Cols
	if rows < 1 {
		rows = 1
	}
	if cols < 1 {
		cols = 1
	}
	return cellSpan{o.Address, rows, cols, bits}
}

func (s cellSpan) end() uint64 {
	return uint64(s.addr) + uint64(s.rows*s.cols*s.bits/8)
}

func sameSpan(os []*Object, in *Object) []*Object {
	want := spanOf(in)
	var out []*Object
	for _, o := range os {
		if spanOf(o) == want {
			out = append(out, o)
		}
	}
	return out
}

func overlapIndex(os []*Object, in *Object) int {
	a := spanOf(in)
	for i, o := range os {
		b := spanOf(o)
		if a == b {
			continue
		}
		if uint64(a.addr) < b.end() && uint64(b.addr) < a.end() {
			return i
		}
	}
	return -1
}

func sameLayout(a, b *Object) bool {
	if a.Data.Signed != b.Data.Signed || a.Data.Endian != b.Data.Endian || a.Data.Float != b.Data.Float || a.Inverse != b.Inverse {
		return false
	}
	return sameAxisLayout(a.X, b.X) && sameAxisLayout(a.Y, b.Y)
}

func sameAxisLayout(a, b *Axis) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Stored != b.Stored {
		return false
	}
	if (a.Address == nil) != (b.Address == nil) {
		return false
	}
	if a.Address != nil && *a.Address != *b.Address {
		return false
	}
	ab, bb := 0, 0
	if a.Data != nil {
		ab = a.Data.Bits
	}
	if b.Data != nil {
		bb = b.Data.Bits
	}
	return ab == bb
}

func sameDisplay(a, b *Object) bool {
	if a.Value.Units != b.Value.Units || a.Value.Conversion != b.Value.Conversion || a.Value.Precision != b.Value.Precision || a.View != b.View {
		return false
	}
	return sameAxisDisplay(a.X, b.X) && sameAxisDisplay(a.Y, b.Y)
}

func sameAxisDisplay(a, b *Axis) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Value.Units == b.Value.Units && a.Value.Conversion == b.Value.Conversion && a.Value.Precision == b.Value.Precision && a.View == b.View
}

func fold(os []*Object, actions []Action) ([]*Object, []Action) {
	var kept []*Object
	for _, o := range os {
		placed := false
		for _, k := range kept {
			if spanOf(k) != spanOf(o) || !sameLayout(k, o) {
				continue
			}
			actions = placeSmart(actions, k, o, nameOf(o))
			placed = true
			break
		}
		if !placed {
			kept = append(kept, o)
		}
	}
	return kept, actions
}

func placeSmart(actions []Action, dst, src *Object, key string) []Action {
	if sameDisplay(dst, src) {
		added := false
		for _, id := range src.Categories {
			if !slices.Contains(dst.Categories, id) {
				dst.Categories = append(dst.Categories, id)
				added = true
			}
		}
		if !added {
			return append(actions, Action{Action: "skipped", Key: key})
		}
		return append(actions, Action{Action: "categories", Key: key})
	}
	alt := alternateOf(dst, src)
	if hasAlternate(dst, alt) {
		return append(actions, Action{Action: "skipped", Key: key, Field: "alternate"})
	}
	dst.Alternates = append(dst.Alternates, alt)
	return append(actions, Action{Action: "alternate", Key: key})
}

func alternateOf(primary, o *Object) Alternate {
	a := Alternate{
		ID: o.ID, Description: o.Description,
		Categories: append([]int{}, o.Categories...),
		Value:      o.Value, View: o.View,
	}
	if o.X != nil && (primary.X == nil || !sameAxisDisplay(primary.X, o.X)) {
		v := o.X.Value
		a.X = &v
	}
	if o.Y != nil && (primary.Y == nil || !sameAxisDisplay(primary.Y, o.Y)) {
		v := o.Y.Value
		a.Y = &v
	}
	return a
}

func hasAlternate(o *Object, a Alternate) bool {
	for _, e := range o.Alternates {
		if e.Value.Units == a.Value.Units && e.Value.Conversion == a.Value.Conversion {
			return true
		}
	}
	return false
}
