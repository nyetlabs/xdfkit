package model

import (
	"bytes"
	"encoding/json"
	"reflect"

	"go.nyet.org/xdfkit/canon"
)

// residue returns the fields of the source record full that differ from
// base, the record rebuilt from the model alone, in kp's JSON form. Objects
// are compared field by field, everything else as a whole; a field missing
// from full is null.
func residue(full, base any) (map[string]any, error) {
	ft, err := tree(full)
	if err != nil {
		return nil, err
	}
	bt, err := tree(base)
	if err != nil {
		return nil, err
	}
	d, _ := diff(ft, bt)
	return d.(map[string]any), nil
}

// restore decodes into v the record rebuilt from the model (base) with the
// residue laid over it.
func restore(v, base any, res map[string]any) error {
	bt, err := tree(base)
	if err != nil {
		return err
	}
	return remarshal(merge(bt, res), v)
}

// remarshal decodes the generic value t into v through JSON.
func remarshal(t, v any) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return canon.Unmarshal(b, v)
}

// tree is v's canonical JSON as a generic value, numbers as json.Number.
func tree(v any) (any, error) {
	b, err := canon.Marshal(v)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var t any
	return t, d.Decode(&t)
}

func diff(full, base any) (any, bool) {
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
		} else if d, changed := diff(fv, bv); changed {
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

func merge(base any, res any) any {
	rm, rok := res.(map[string]any)
	bm, bok := base.(map[string]any)
	if !rok || !bok {
		return res
	}
	out := make(map[string]any, len(bm)+len(rm))
	for k, v := range bm {
		out[k] = v
	}
	for k, v := range rm {
		out[k] = merge(bm[k], v)
	}
	return out
}
