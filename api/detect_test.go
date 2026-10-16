package api

import (
	"testing"

	"go.nyet.org/xdfkit/canon"
	"go.nyet.org/xdfkit/model"
)

func TestDetectA2LAndDAMOS(t *testing.T) {
	if Detect([]byte("ASAP2_VERSION 1 31\n")) != A2L {
		t.Fatal("a2l")
	}
	if Detect([]byte("*** Created by ASAP2DAM Version 6.05 ***\n")) != DAMOS {
		t.Fatal("damos")
	}
	if Detect([]byte("schema: xdfkit-model/1\n")) != YAML {
		t.Fatal("yaml")
	}
	a2l := []byte(`
ASAP2_VERSION 1 31
/begin PROJECT P ""
/begin MODULE M ""
/begin MOD_COMMON ""
BYTE_ORDER MSB_LAST
/end MOD_COMMON
/begin COMPU_METHOD ident ""
RAT_FUNC "%3.0" ""
COEFFS 0 1 0 0 0 1
/end COMPU_METHOD
/begin RECORD_LAYOUT Scalar
FNC_VALUES 1 UBYTE
/end RECORD_LAYOUT
/begin CHARACTERISTIC V "one"
VALUE 0x800010 Scalar 0 ident 0 1
/end CHARACTERISTIC
/end MODULE
/end PROJECT
`)
	out, resp, err := Convert(a2l, ConvertRequest{To: JSON, Name: "t.a2l"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.From != A2L {
		t.Fatalf("from %s", resp.From)
	}
	var m model.Model
	if err := canon.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 1 || m.Objects[0].Address != 0x10 || m.Provenance.Format != A2L {
		t.Fatalf("%+v", m.Objects[0])
	}
	dam := []byte("*** Created by ASAP2DAM ***\n/EPR, {P}, {1}, X\n1, /SPZ, V, {one}, 1, $800020, $800020\n/SPW, 0, 1, 0, 1\n")
	out, resp, err = Convert(dam, ConvertRequest{To: JSON, Name: "t.dam"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.From != DAMOS {
		t.Fatalf("from %s", resp.From)
	}
	m = model.Model{}
	if err := canon.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 1 || m.Objects[0].Address != 0x20 || m.Provenance.Origin != DAMOS {
		t.Fatalf("%+v", m)
	}
}
