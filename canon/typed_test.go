package canon

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

type inner struct {
	Cols  uint8   `json:"cols"`
	Shift int16   `json:"shift"`
	Gain  float32 `json:"gain"`
}

type Base struct {
	ID string `json:"id"`
}

type typed struct {
	*Base
	Factor float64           `json:"factor"`
	Count  int64             `json:"count"`
	Maps   []inner           `json:"maps"`
	Pair   [2]int32          `json:"pair"`
	Opt    *float64          `json:"opt,omitempty"`
	Zero   inner             `json:"zero,omitzero"`
	Names  map[string]uint16 `json:"names"`
	Raw    []byte            `json:"raw"`
	Skip   int               `json:"-"`
}

func TestTypedWrite(t *testing.T) {
	v := typed{
		Base:   &Base{ID: "x"},
		Factor: -30,
		Count:  1 << 62,
		Maps:   []inner{{Cols: 16, Shift: -1, Gain: 0.1}, {Gain: 2}},
		Pair:   [2]int32{1, -2},
		Names:  map[string]uint16{"a": 7},
		Raw:    []byte{1, 2},
		Skip:   9,
	}
	got, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"factor": -30.0`, `"count": 4611686018427387904`, `"gain": 0.1`, `"gain": 2.0`, `"cols": 16`, `"id": "x"`, `"raw": "AQI="`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	for _, absent := range []string{`"opt"`, `"zero"`, `"Skip"`, `"skip"`, `"Base"`} {
		if bytes.Contains(got, []byte(absent)) {
			t.Errorf("unexpected %s in\n%s", absent, got)
		}
	}
	if want := jq(t, got, canonArgs...); !bytes.Equal(got, want) {
		t.Errorf("not a jq fixed point:\n%s\nwant\n%s", got, want)
	}
	var back typed
	if err := Unmarshal(got, &back); err != nil {
		t.Fatal(err)
	}
	if again, _ := Marshal(back); !bytes.Equal(again, got) {
		t.Errorf("round trip differs:\n%s\nwant\n%s", again, got)
	}
}

func TestWholeFloats(t *testing.T) {
	got, err := Marshal([]float64{0, math.Copysign(0, -1), 1, -30, 1e21, 1.5e300, 0.5, 4.7e-7})
	if err != nil {
		t.Fatal(err)
	}
	if want := jq(t, got, canonArgs...); !bytes.Equal(got, want) {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTypedRead(t *testing.T) {
	ok := []string{
		`{"maps": [{"cols": 16}]}`,
		`{"maps": [{"cols": 16.0}]}`,
		`{"maps": [{"cols": 1.6E1}]}`,
		`{"maps": [{"cols": 160e-1}]}`,
		`{"count": 9007199254740993.0}`,
		`{"factor": 1, "stamp": {"anything": true}}`,
		`{"id": "x"}`,
	}
	for _, in := range ok {
		var v typed
		if err := Unmarshal([]byte(in), &v); err != nil {
			t.Errorf("%s: %v", in, err)
		}
	}
	var v typed
	_ = Unmarshal([]byte(`{"count": 9007199254740993.0, "maps": [{"cols": 1.6E1}], "id": "x"}`), &v)
	if v.Count != 9007199254740993 || v.Maps[0].Cols != 16 || v.Base == nil || v.ID != "x" {
		t.Errorf("decoded %+v", v)
	}
	bad := map[string]string{
		`{"maps": [{}, {"cols": 16.5}]}`:       "maps[1].cols: 16.5 is not an integer",
		`{"maps": [{"cols": 256}]}`:            "maps[0].cols: 256 is out of range for uint8",
		`{"maps": [{"shift": -32769}]}`:        "maps[0].shift: -32769 is out of range for int16",
		`{"maps": [{"cols": -1}]}`:             "maps[0].cols: -1 is out of range for uint8",
		`{"maps": [{"cols": "16"}]}`:           "maps[0].cols: expected an integer, got a string",
		`{"maps": [{"rows": 1}]}`:              "maps[0].rows: unknown field",
		`{"pair": [1]}`:                        "pair: expected 2 elements, got 1",
		`{"maps": {"cols": 1}}`:                "maps: expected an array, got an object",
		`{"names": {"a": 1.5}}`:                "names.a: 1.5 is not an integer",
		`{"maps": [{"gain": 1e39}]}`:           "maps[0].gain: 1e39 is out of range for float32",
		`{"nested": {"stamp": {}}}`:            "nested: unknown field",
		`[1]`:                                  ".: expected an object, got an array",
		`{"maps": [{"cols": 1}]} {"extra": 1}`: "data after the JSON document",
	}
	for in, want := range bad {
		var v typed
		err := Unmarshal([]byte(in), &v)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", in, err, want)
		}
	}
}
