package xdf

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
)

// MetaSchemaID names the schema version of the metadata file.
const MetaSchemaID = "xdfkit-xdf-meta/1"

// metaFile is the metadata file written next to an XDF
// (docs/stamp-and-metadata.md, Metadata file): JSON Merge Patches from the
// XDF view to the model, objects keyed by XDF uniqueid, and the digests of
// the XDF view (without object keys) in each canonical form.
type metaFile struct {
	Schema string `json:"schema"`
	XDF    struct {
		Digests []canon.Digest `json:"digests"`
	} `json:"xdf"`
	Model   map[string]any            `json:"model,omitempty"`
	Objects map[string]map[string]any `json:"objects"`
}

// Meta returns the metadata file for m written as the XDF x (by Write), with
// a stamp naming tool.
func Meta(m *model.Model, x []byte, tool string) ([]byte, error) {
	v, uids, _, err := view(x)
	if err != nil {
		return nil, err
	}
	if len(v.Objects) != len(m.Objects) {
		return nil, fmt.Errorf("the XDF has %d objects, the model %d", len(v.Objects), len(m.Objects))
	}
	vh, vobjs, err := split(v, false)
	if err != nil {
		return nil, err
	}
	mh, mobjs, err := split(m, true)
	if err != nil {
		return nil, err
	}
	mf := metaFile{
		Schema:  MetaSchemaID,
		Model:   canon.MergeDiff(vh, mh).(map[string]any),
		Objects: map[string]map[string]any{},
	}
	if mf.XDF.Digests, err = canon.Digests(whole(vh, vobjs)); err != nil {
		return nil, err
	}
	for i, uid := range uids {
		if uid != hex(i+1) {
			return nil, fmt.Errorf("object %d has uniqueid %s: the XDF wasn't written from this model", i, uid)
		}
		mf.Objects[uid] = canon.MergeDiff(vobjs[i], mobjs[i]).(map[string]any)
	}
	return canon.MarshalStamped(mf, tool)
}

// Read reads an XDF, merged with its metadata file unless meta is nil. With
// the XDF unchanged since Meta, the result is the model it was written from.
// Otherwise objects take the XDF's values and keep the metadata's where XDF
// can't express them; objects new in the XDF have no metadata, and objects
// missing from it are dropped. Warnings report XDF content the reader ignores
// and the edits.
func Read(x, meta []byte) (*model.Model, []string, error) {
	v, uids, warnings, err := view(x)
	if err != nil || meta == nil {
		return v, warnings, err
	}
	s, _, err := canon.Verify(meta)
	if err != nil {
		return nil, warnings, fmt.Errorf("metadata: %w", err)
	}
	if s != canon.Clean && s != canon.Unstamped {
		warnings = append(warnings, "metadata stamp "+s.String())
	}
	var mf metaFile
	if err := canon.Unmarshal(meta, &mf); err != nil {
		return nil, warnings, fmt.Errorf("metadata: %w", err)
	}
	if mf.Schema != MetaSchemaID {
		return nil, warnings, fmt.Errorf("metadata: schema %q, want %q", mf.Schema, MetaSchemaID)
	}
	m, ws, err := merge(v, uids, &mf)
	return m, append(warnings, ws...), err
}

func merge(v *model.Model, uids []string, mf *metaFile) (*model.Model, []string, error) {
	vh, vobjs, err := split(v, false)
	if err != nil {
		return nil, nil, err
	}
	ds, err := canon.Digests(whole(vh, vobjs))
	if err != nil {
		return nil, nil, err
	}
	edited := !slices.Equal(ds, mf.XDF.Digests)
	m := new(model.Model)
	if err := canon.FromTree(canon.MergePatch(vh, mf.Model), m); err != nil {
		return nil, nil, fmt.Errorf("metadata model: %w", err)
	}
	var warnings []string
	if edited {
		warnings = append(warnings, "XDF edited since its metadata file was written")
		next := 0
		for _, c := range m.Categories {
			next = max(next, c.ID+1)
		}
		for _, c := range v.Categories {
			if !slices.ContainsFunc(m.Categories, func(d *model.Category) bool { return trim(d.Name) == c.Name }) {
				m.Categories = append(m.Categories, &model.Category{ID: next, Name: c.Name})
				next++
			}
		}
	}
	_, catIndex := categories(m)
	written := func(ids []int) int { // the XDF category the writer gives ids
		if len(ids) == 0 {
			return 0
		}
		return catIndex[ids[0]]
	}
	m.Objects = []*model.Object{}
	seen := map[string]bool{}
	for i, vo := range v.Objects {
		uid := uids[i]
		name := fmt.Sprintf("%s %q", uid, vo.ID)
		p, ok := mf.Objects[uid]
		if !ok || seen[uid] {
			warnings = append(warnings, name+": not in the metadata file")
			o := *vo
			o.Key, o.Categories = "", remap(vo.Categories, v.Categories, m.Categories)
			m.Objects = append(m.Objects, &o)
			continue
		}
		seen[uid] = true
		c := new(model.Object)
		if err := canon.FromTree(canon.MergePatch(vobjs[i], p), c); err != nil {
			return nil, nil, fmt.Errorf("metadata object %s: %w", uid, err)
		}
		if !edited {
			m.Objects = append(m.Objects, c)
			continue
		}
		o := reconcile(vo, c)
		o.Categories = remap(vo.Categories, v.Categories, m.Categories)
		if len(o.Categories) > 0 && written(o.Categories) == written(c.Categories) {
			o.Categories = c.Categories
		}
		if ps, err := changed(c, o); err != nil {
			return nil, nil, err
		} else if len(ps) > 0 {
			warnings = append(warnings, name+": metadata not applied to "+strings.Join(ps, ", "))
		}
		m.Objects = append(m.Objects, o)
	}
	for _, uid := range slices.Sorted(maps.Keys(mf.Objects)) {
		if !seen[uid] {
			warnings = append(warnings, uid+": in the metadata file but not in the XDF; dropped")
		}
	}
	m.AssignKeys()
	return m, warnings, m.Check()
}

// reconcile returns an object of an edited XDF: the XDF's values (v), with
// the metadata's (c) where XDF can't express them or where they lower to
// what the XDF holds. The caller sets the categories.
func reconcile(v, c *model.Object) *model.Object {
	o := *v
	o.Key, o.Inverse, o.View = c.Key, c.Inverse, c.View
	if t, d := text(c); t == v.ID && d == v.Description {
		o.ID, o.Description, o.Comment = c.ID, c.Description, c.Comment
	}
	if (c.Shape == "value") == (v.Shape == "value") && (v.Shape == "value" || max(c.Rows, 1) == v.Rows && max(c.Cols, 1) == v.Cols) {
		o.Shape, o.Rows, o.Cols = c.Shape, c.Rows, c.Cols
	}
	value(&o.Value, c.Value, o.Data)
	if o.Shape == "value" {
		o.X, o.Y = c.X, c.Y
	} else {
		o.X, o.Y = reconcileAxis(v.X, c.X), reconcileAxis(v.Y, c.Y)
	}
	return &o
}

// reconcileAxis is reconcile for an axis.
func reconcileAxis(v, c *model.Axis) *model.Axis {
	var o model.Axis
	switch {
	case c == nil:
		if v == nil || v.Source != "image" && v.Value == (model.Value{Conversion: model.Conversion{Factor: 1}}) {
			return nil
		}
		return v
	case v == nil:
		if labels(c) {
			return c
		}
		return nil
	case v.Source == "image" && c.Source == "image" && c.Stored != "backwards":
		o = *v
		o.Stored, o.Mirror, o.Header, o.Signature = c.Stored, c.Mirror, c.Header, c.Signature
	case v.Source != "image" && (labels(c) || c.Stored == "subtract"):
		o = *c
		o.Value.Units = v.Value.Units
		o.Value.Precision = v.Value.Precision
	default:
		return v
	}
	o.View = c.View
	var d model.Data
	if o.Data != nil {
		d = *o.Data
	}
	value(&o.Value, c.Value, d)
	return &o
}

// value sets o's description from c, and c's conversion and precision where
// the writer would turn them into what the XDF holds (o).
func value(o *model.Value, c model.Value, d model.Data) {
	o.Description = c.Description
	if trim(c.Units) == o.Units {
		o.Units = c.Units
	}
	if formula(c.Conversion) == formula(o.Conversion) {
		o.Conversion = c.Conversion
	}
	p := *o
	p.Precision = c.Precision
	if limitPrecision(p, d.Bits, d.Signed) == o.Precision {
		o.Precision = c.Precision
	}
}

// remap translates category ids from one category list to another by name,
// as the writer trims it.
func remap(ids []int, from, to []*model.Category) []int {
	if slices.EqualFunc(from, to, func(a, b *model.Category) bool { return *a == *b }) {
		return ids
	}
	var out []int
	for _, id := range ids {
		i := slices.IndexFunc(from, func(c *model.Category) bool { return c.ID == id })
		if i < 0 {
			continue
		}
		if j := slices.IndexFunc(to, func(c *model.Category) bool { return trim(c.Name) == trim(from[i].Name) }); j >= 0 {
			out = append(out, to[j].ID)
		}
	}
	return out
}

// changed returns the JSON paths where o differs from c: the metadata values
// that reconcile replaced with the XDF's.
func changed(c, o *model.Object) ([]string, error) {
	ct, err := canon.Tree(c)
	if err != nil {
		return nil, err
	}
	ot, err := canon.Tree(o)
	if err != nil {
		return nil, err
	}
	var out []string
	var walk func(t any, path string)
	walk = func(t any, path string) {
		m, ok := t.(map[string]any)
		if !ok || len(m) == 0 {
			out = append(out, path)
			return
		}
		for _, k := range slices.Sorted(maps.Keys(m)) {
			p := k
			if path != "" {
				p = path + "." + k
			}
			walk(m[k], p)
		}
	}
	if d := canon.MergeDiff(ct, ot).(map[string]any); len(d) > 0 {
		walk(d, "")
	}
	return out, nil
}

// split returns m's tree without its objects, and each object's tree, without
// its key unless keys: view keys depend on the other objects' titles, so they
// aren't part of the view.
func split(m *model.Model, keys bool) (map[string]any, []any, error) {
	t, err := canon.Tree(m)
	if err != nil {
		return nil, nil, err
	}
	h, ok := t.(map[string]any)
	if !ok {
		return nil, nil, errors.New("model is not an object")
	}
	objs, _ := h["objects"].([]any)
	for _, o := range objs {
		if o, ok := o.(map[string]any); ok && !keys {
			delete(o, "key")
		}
	}
	delete(h, "objects")
	return h, objs, nil
}

// whole is the view tree from its parts (see split).
func whole(h map[string]any, objs []any) map[string]any {
	w := maps.Clone(h)
	w["objects"] = objs
	return w
}
