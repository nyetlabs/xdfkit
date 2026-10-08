package canon

import (
	"strings"
	"testing"
)

func TestYAMLRoundTrip(t *testing.T) {
	in := `{"a":1,"b":1.0,"c":1e-07,"d":"0x1F","e":"true","f":"null","g":"1.5","h":"","i":null,"j":[true,false],"k":{"y":"-","z":"a: b"}}`
	y, err := JSONToYAML([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"a: 1\n", "b: 1.0\n", "c: 1e-07\n", `d: "0x1F"`, `e: "true"`, `f: "null"`, `g: "1.5"`} {
		if !strings.Contains(string(y), want) {
			t.Errorf("yaml lacks %q:\n%s", want, y)
		}
	}
	back, err := YAMLToJSON(y)
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != in {
		t.Errorf("round trip:\n got %s\nwant %s", back, in)
	}
}

func TestYAMLToJSON(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"a: 0x1F\nb: 1_000\nc: ~\nd: 2001-01-01\ne: !!str 5\n", `{"a":31,"b":1000,"c":null,"d":"2001-01-01","e":"5"}`},
		{"[1.50, -0, 16.0]", `[1.50,-0,16.0]`},
	} {
		got, err := YAMLToJSON([]byte(c.in))
		if err != nil || string(got) != c.want {
			t.Errorf("%q: got %s, %v; want %s", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{
		"a: &x 1\nb: *x\n",
		"a: 1\n---\nb: 2\n",
		"a: !custom 1\n",
		"1: a\n",
		"a: .inf\n",
		"a: .nan\n",
		"# only a comment\n",
	} {
		if got, err := YAMLToJSON([]byte(in)); err == nil {
			t.Errorf("%q: got %s, want an error", in, got)
		}
	}
}
