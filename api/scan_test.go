package api

import (
	"encoding/json"
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"
)

func TestConvertJSONExtremes(t *testing.T) {
	m := &model.Model{
		Schema: model.SchemaID,
		Objects: []*model.Object{{
			Key: "t", ID: "T", Address: 0, Rows: 1, Cols: 2, Shape: "1d",
			Data: model.Data{Bits: 8}, Value: model.Value{Conversion: model.Conversion{Factor: 2}},
		}},
	}
	in, _ := json.Marshal(m)
	out, _, err := Convert(in, ConvertRequest{To: JSON, Name: "t.json"})
	if err != nil || strings.Contains(string(out), `"extremes"`) {
		t.Fatalf("no image: %v\n%s", err, out)
	}
	out, _, err = Convert(in, ConvertRequest{To: JSON, Name: "t.json", Image: []byte{0x01, 0x04}})
	if err != nil {
		t.Fatal(err)
	}
	var got model.Model
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	e := got.Objects[0].Extremes
	if e == nil || e.Min != 2 || e.Max != 8 || e.RawMin != "0x1" || e.RawMax != "0x4" {
		t.Fatalf("%+v", e)
	}
}
