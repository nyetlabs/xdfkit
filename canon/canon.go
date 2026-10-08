// Package canon prints JSON exactly as `jq -S .` (jq 1.7 or later) does, and
// computes and checks the edit stamp described in docs/stamp-and-metadata.md.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
)

const (
	StampKey = "stamp"
	indent   = "  "
)

// Canonical forms a stamp digest can be computed over.
const (
	CanonJCS = "RFC8785"
	CanonJq  = "jq -S ."
)

// canonical renders a document (without its stamp) in each canonical form.
var canonical = []struct {
	name string
	text func(t any) ([]byte, error)
}{
	{CanonJCS, func(t any) ([]byte, error) {
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		return jcs.Transform(b)
	}},
	{CanonJq, func(t any) ([]byte, error) { return render(t), nil }},
}

// Stamp is the top-level "stamp" object: one digest per canonical form.
type Stamp struct {
	Digests []Digest `json:"digests"`
	Tool    string   `json:"tool"`
}

// Digest is "sha256:<hex>" over the document, without its stamp, in form Canon.
type Digest struct {
	Canon  string `json:"canon"`
	Digest string `json:"digest"`
}

// Marshal returns the canonical text of v.
func Marshal(v any) ([]byte, error) {
	t, err := tree(v)
	if err != nil {
		return nil, err
	}
	return render(t), nil
}

// MarshalStamped returns the canonical text of v, which must encode to a JSON
// object, with a top-level stamp over the rest of the document.
func MarshalStamped(v any, tool string) ([]byte, error) {
	t, err := tree(v)
	if err != nil {
		return nil, err
	}
	obj, ok := t.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canon: stamped document must be an object")
	}
	delete(obj, StampKey)
	var ds []any
	for _, c := range canonical {
		d, err := digest(c.text, obj)
		if err != nil {
			return nil, err
		}
		ds = append(ds, map[string]any{"canon": c.name, "digest": d})
	}
	obj[StampKey] = map[string]any{"digests": ds, "tool": tool}
	return render(obj), nil
}

// Status is the result of Verify, overall or for one digest.
type Status int

const (
	Clean     Status = iota // digest matches
	Edited                  // digest differs
	Mixed                   // overall: some digests match, some don't
	Unknown                 // canonical form or digest algorithm not supported
	Unstamped               // overall: no stamp
)

func (s Status) String() string {
	return [...]string{"clean", "edited", "mixed", "unknown", "unstamped"}[s]
}

// Check is the result for one digest of a stamp.
type Check struct {
	Digest
	Status Status
}

// Verify checks every digest in the stamp of a JSON document. Whitespace and
// key order never matter. Under "jq -S ." number literals are compared as
// written, as jq 1.7+ keeps them; RFC 8785 compares numbers by value.
func Verify(data []byte) (Status, []Check, error) {
	t, err := decode(data)
	if err != nil {
		return 0, nil, err
	}
	obj, ok := t.(map[string]any)
	if !ok {
		return Unstamped, nil, nil
	}
	raw, ok := obj[StampKey]
	if !ok {
		return Unstamped, nil, nil
	}
	b, _ := json.Marshal(raw)
	var s Stamp
	if err := json.Unmarshal(b, &s); err != nil {
		return 0, nil, fmt.Errorf("canon: bad stamp: %w", err)
	}
	delete(obj, StampKey)
	var checks []Check
	seen := map[Status]bool{}
	for _, d := range s.Digests {
		ck := Check{d, Unknown}
		for _, c := range canonical {
			if c.name != d.Canon || !strings.HasPrefix(d.Digest, "sha256:") {
				continue
			}
			want, err := digest(c.text, obj)
			if err != nil {
				return 0, nil, err
			}
			ck.Status = map[bool]Status{true: Clean, false: Edited}[want == d.Digest]
		}
		checks = append(checks, ck)
		seen[ck.Status] = true
	}
	switch {
	case seen[Clean] && seen[Edited]:
		return Mixed, checks, nil
	case seen[Clean]:
		return Clean, checks, nil
	case seen[Edited]:
		return Edited, checks, nil
	}
	return Unknown, checks, nil
}

func digest(text func(any) ([]byte, error), t any) (string, error) {
	b, err := text(t)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// tree converts v into maps, slices, strings, bools, nil and json.Number, with
// numbers spelled by type as jq prints them.
func tree(v any) (any, error) { return encode(reflect.ValueOf(v)) }

func decode(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var t any
	if err := d.Decode(&t); err != nil {
		return nil, err
	}
	if d.More() {
		return nil, fmt.Errorf("canon: data after the JSON document")
	}
	return t, nil
}

// respell rewrites exponent literals from MarshalJSON output and json.Number
// values the way jq 1.8 prints them: plain digits at 1e21 and above, and
// shortest digits with "E-n" below 1e-6.
func respell(n json.Number) (json.Number, error) {
	s := string(n)
	if !strings.ContainsAny(s, "eE") {
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return n, err
	}
	if math.Abs(f) >= 1 {
		return json.Number(strconv.FormatFloat(f, 'f', -1, 64)), nil
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, 64), "e-")
	return json.Number(mant + "E-" + strings.TrimLeft(exp, "0")), nil
}

func normalize(t any) (any, error) {
	var err error
	switch t := t.(type) {
	case json.Number:
		return respell(t)
	case map[string]any:
		for k, v := range t {
			if t[k], err = normalize(v); err != nil {
				return nil, err
			}
		}
	case []any:
		for i, v := range t {
			if t[i], err = normalize(v); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

func render(t any) []byte {
	var b bytes.Buffer
	write(&b, t, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func write(b *bytes.Buffer, t any, depth int) {
	nl := func(d int) {
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(indent, d))
	}
	switch t := t.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		fmt.Fprint(b, t)
	case json.Number:
		b.WriteString(string(t))
	case string:
		writeString(b, t)
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, v := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			write(b, v, depth+1)
		}
		nl(depth)
		b.WriteByte(']')
	case map[string]any:
		if len(t) == 0 {
			b.WriteString("{}")
			return
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			writeString(b, k)
			b.WriteString(": ")
			write(b, t[k], depth+1)
		}
		nl(depth)
		b.WriteByte('}')
	default:
		panic(fmt.Sprintf("canon: unexpected %T", t))
	}
}

// writeString escapes like jq: quote, backslash, and control characters
// (named where JSON has a short form, else \u00xx), including DEL.
func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}
