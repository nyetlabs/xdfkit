package canon

import (
	"fmt"
	"reflect"
)

// Tree returns v as Marshal encodes it: a generic tree of maps, slices,
// strings, bools, nil and json.Number spelled as jq prints it.
func Tree(v any) (any, error) { return tree(v) }

// FromTree decodes the generic tree t into v, as Unmarshal does.
func FromTree(t, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("canon: decoding needs a non-nil pointer, not %T", v)
	}
	return assign(t, rv.Elem(), "")
}

// Digests returns the digest of v, which must not hold a stamp, in each
// canonical form: RFC 8785 first, then jq -S .
func Digests(v any) ([]Digest, error) {
	t, err := tree(v)
	if err != nil {
		return nil, err
	}
	var ds []Digest
	for _, c := range canonical {
		d, err := digest(c.text, t)
		if err != nil {
			return nil, err
		}
		ds = append(ds, Digest{c.name, d})
	}
	return ds, nil
}

// MergePatch applies a JSON Merge Patch (RFC 7396) to the generic tree
// target: objects merge key by key, null removes a key, anything else
// replaces. The trees are not modified.
func MergePatch(target, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	tm, _ := target.(map[string]any)
	out := make(map[string]any, len(tm)+len(pm))
	for k, v := range tm {
		out[k] = v
	}
	for k, v := range pm {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = MergePatch(out[k], v)
		}
	}
	return out
}

// MergeDiff returns the merge patch that turns base into full, both generic
// trees without nulls: objects are compared key by key, everything else as a
// whole. Equal objects give an empty patch.
func MergeDiff(base, full any) any {
	d, _ := mergeDiff(base, full)
	return d
}

func mergeDiff(base, full any) (any, bool) {
	fm, fok := full.(map[string]any)
	bm, bok := base.(map[string]any)
	if !fok || !bok {
		return full, !reflect.DeepEqual(full, base)
	}
	out := map[string]any{}
	for k, fv := range fm {
		bv, ok := bm[k]
		if !ok {
			out[k] = fv
		} else if d, changed := mergeDiff(bv, fv); changed {
			out[k] = d
		}
	}
	for k := range bm {
		if _, ok := fm[k]; !ok {
			out[k] = nil
		}
	}
	return out, len(out) > 0
}
