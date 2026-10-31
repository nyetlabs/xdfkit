package kp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// noisePaths are container/export noise skipped by Diff unless keepNoise.
// Matched as a path segment (filename, project.trailing).
var noisePaths = []string{
	"filename", "headerLen", "endOffset", "headerBody", "trailing", "fixedHeader",
}

// Flat returns every leaf of f's JSON under a dotted path (maps[0].id).
func Flat(f *File) (map[string]string, error) {
	j, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(j, &v); err != nil {
		return nil, err
	}
	out := map[string]string{}
	walkFlat("", v, out)
	return out, nil
}

func walkFlat(p string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			np := k
			if p != "" {
				np = p + "." + k
			}
			walkFlat(np, x, out)
		}
	case []any:
		for i, x := range t {
			walkFlat(fmt.Sprintf("%s[%d]", p, i), x, out)
		}
	default:
		b, _ := json.Marshal(t)
		s := string(b)
		if len(s) > 96 {
			s = fmt.Sprintf("%s... (%d)", s[:48], len(s))
		}
		out[p] = s
	}
}

// DiffLine is one field that differs between two Flats.
type DiffLine struct {
	Path string
	A, B string // empty means missing
}

// Diff compares two Flat maps. Paths in noisePaths are skipped unless keepNoise.
func Diff(a, b map[string]string, keepNoise bool) []DiffLine {
	keys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		keys[k] = struct{}{}
	}
	for k := range b {
		keys[k] = struct{}{}
	}
	ks := make([]string, 0, len(keys))
	for k := range keys {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	out := make([]DiffLine, 0)
	for _, k := range ks {
		if !keepNoise && isNoise(k) {
			continue
		}
		va, oka := a[k]
		vb, okb := b[k]
		if oka && okb && va == vb {
			continue
		}
		if !oka {
			va = ""
		}
		if !okb {
			vb = ""
		}
		out = append(out, DiffLine{Path: k, A: va, B: vb})
	}
	return out
}

func isNoise(path string) bool {
	for _, n := range noisePaths {
		if path == n || strings.HasPrefix(path, n+".") || strings.HasPrefix(path, n+"[") {
			return true
		}
		if i := strings.Index(path, "."+n); i >= 0 {
			rest := path[i+1+len(n):]
			if rest == "" || rest[0] == '.' || rest[0] == '[' {
				return true
			}
		}
	}
	return false
}

// ByteDiff lists offsets where a and b differ, up to limit (0 means all).
func ByteDiff(a, b []byte, limit int) []string {
	n := min(len(a), len(b))
	var out []string
	for i := range n {
		if a[i] == b[i] {
			continue
		}
		out = append(out, fmt.Sprintf("0x%X: %02x -> %02x", i, a[i], b[i]))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(a) != len(b) && (limit <= 0 || len(out) < limit) {
		out = append(out, fmt.Sprintf("length: %d -> %d", len(a), len(b)))
	}
	return out
}
