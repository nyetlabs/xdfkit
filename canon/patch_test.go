package canon

import (
	"reflect"
	"testing"
)

// TestMergePatch runs the examples of RFC 7396, appendix A, and checks that
// MergeDiff finds a patch that MergePatch undoes.
func TestMergePatch(t *testing.T) {
	for _, c := range []struct{ target, patch, want string }{
		{`{"a":"b"}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"b"}`, `{"b":"c"}`, `{"a":"b","b":"c"}`},
		{`{"a":"b"}`, `{"a":null}`, `{}`},
		{`{"a":"b","b":"c"}`, `{"a":null}`, `{"b":"c"}`},
		{`{"a":["b"]}`, `{"a":"c"}`, `{"a":"c"}`},
		{`{"a":"c"}`, `{"a":["b"]}`, `{"a":["b"]}`},
		{`{"a":{"b":"c"}}`, `{"a":{"b":"d","c":null}}`, `{"a":{"b":"d"}}`},
		{`{"a":[{"b":"c"}]}`, `{"a":[1]}`, `{"a":[1]}`},
		{`["a","b"]`, `["c","d"]`, `["c","d"]`},
		{`{"a":"b"}`, `["c"]`, `["c"]`},
		{`{"e":null}`, `{"a":1}`, `{"e":null,"a":1}`},
		{`[1,2]`, `{"a":"b","c":null}`, `{"a":"b"}`},
		{`{}`, `{"a":{"bb":{"ccc":null}}}`, `{"a":{"bb":{}}}`},
	} {
		target, _ := decode([]byte(c.target))
		patch, _ := decode([]byte(c.patch))
		want, _ := decode([]byte(c.want))
		if got := MergePatch(target, patch); !reflect.DeepEqual(got, want) {
			t.Errorf("MergePatch(%s, %s) = %v, want %s", c.target, c.patch, got, c.want)
		}
	}
	for _, c := range [][2]string{
		{`{"a":1,"b":{"c":[1],"d":"x"}}`, `{"a":1,"b":{"c":[2],"e":true}}`},
		{`{"a":1}`, `{"a":1}`},
		{`{}`, `{"a":{"b":{}}}`},
	} {
		base, _ := decode([]byte(c[0]))
		full, _ := decode([]byte(c[1]))
		if got := MergePatch(base, MergeDiff(base, full)); !reflect.DeepEqual(got, full) {
			t.Errorf("MergeDiff(%s, %s) doesn't patch back: %v", c[0], c[1], got)
		}
	}
}
