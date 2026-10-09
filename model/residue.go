package model

import "go.nyet.org/xdfkit/canon"

// residue returns the fields of the source record full that differ from
// base, the record rebuilt from the model alone, in kp's JSON form, as a JSON
// Merge Patch: objects are compared field by field, everything else as a
// whole; a field missing from full is null.
func residue(full, base any) (map[string]any, error) {
	ft, err := canon.Tree(full)
	if err != nil {
		return nil, err
	}
	bt, err := canon.Tree(base)
	if err != nil {
		return nil, err
	}
	return canon.MergeDiff(bt, ft).(map[string]any), nil
}

// restore decodes into v the record rebuilt from the model (base) with the
// residue laid over it.
func restore(v, base any, res map[string]any) error {
	bt, err := canon.Tree(base)
	if err != nil {
		return err
	}
	return canon.FromTree(canon.MergePatch(bt, res), v)
}
