package xdf

import (
	"bytes"
	"testing"

	"go.nyet.org/xdfkit/model"
)

func TestReadAbsentTypeFlagsDefaultLittle(t *testing.T) {
	const xdf = `<?xml version="1.0" encoding="UTF-8"?>
<XDFFORMAT version="1.50">
  <XDFHEADER>
    <deftitle>t</deftitle>
    <BASEOFFSET number="0" />
    <DEFAULTS datasizeinbits="8" sigdigits="2" outputtype="1" signed="0" lsbfirst="1" float="0" />
  </XDFHEADER>
  <XDFTABLE uniqueid="0x0">
    <title>ABSENT</title>
    <XDFAXIS id="x">
      <EMBEDDEDDATA mmedaddress="0x100" mmedelementsizebits="16" />
      <indexcount>2</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="y">
      <indexcount>1</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="z">
      <EMBEDDEDDATA mmedtypeflags="0x02" mmedaddress="0x200" mmedelementsizebits="16" mmedrowcount="1" mmedcolcount="2" />
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
  </XDFTABLE>
  <XDFTABLE uniqueid="0x1">
    <title>EXPLICIT_BIG</title>
    <XDFAXIS id="x">
      <EMBEDDEDDATA mmedtypeflags="0x00" mmedaddress="0x300" mmedelementsizebits="16" />
      <indexcount>2</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="y">
      <indexcount>1</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="z">
      <EMBEDDEDDATA mmedtypeflags="0x00" mmedaddress="0x400" mmedelementsizebits="16" mmedrowcount="1" mmedcolcount="2" />
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
  </XDFTABLE>
</XDFFORMAT>
`
	m, _, _, _, err := view([]byte(xdf))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 2 {
		t.Fatalf("objects: %d", len(m.Objects))
	}
	abs := m.Objects[0]
	if abs.X == nil || abs.X.Data == nil || abs.X.Data.Endian != "little" {
		t.Fatalf("absent flags axis: %#v", abs.X)
	}
	if abs.Data.Endian != "little" {
		t.Fatalf("explicit LE z: %#v", abs.Data)
	}
	big := m.Objects[1]
	if big.X == nil || big.X.Data == nil || big.X.Data.Endian != "big" {
		t.Fatalf("explicit big axis: %#v", big.X)
	}
	if big.Data.Endian != "big" {
		t.Fatalf("explicit big z: %#v", big.Data)
	}
}

func TestReadAbsentTypeFlagsNoDefaults(t *testing.T) {
	const xdf = `<?xml version="1.0" encoding="UTF-8"?>
<XDFFORMAT version="1.50">
  <XDFHEADER>
    <deftitle>t</deftitle>
  </XDFHEADER>
  <XDFCONSTANT uniqueid="0x0">
    <title>C</title>
    <EMBEDDEDDATA mmedaddress="0x10" mmedelementsizebits="16" />
    <MATH equation="X"><VAR id="X" /></MATH>
  </XDFCONSTANT>
</XDFFORMAT>
`
	m, _, _, _, err := view([]byte(xdf))
	if err != nil {
		t.Fatal(err)
	}
	if m.Objects[0].Data.Endian != "little" {
		t.Fatalf("no DEFAULTS: %#v", m.Objects[0].Data)
	}
}

// TunerPro omits mmedaddress when it is 0 (confirmed on float-flag save).
func TestReadAbsentAddressIsZero(t *testing.T) {
	const xdf = `<?xml version="1.0" encoding="UTF-8"?>
<XDFFORMAT version="1.50">
  <XDFHEADER>
    <deftitle>t</deftitle>
  </XDFHEADER>
  <XDFCONSTANT uniqueid="0x1">
    <title>F1</title>
    <EMBEDDEDDATA mmedtypeflags="0x10002" mmedelementsizebits="32" />
    <MATH equation="X"><VAR id="X" /></MATH>
  </XDFCONSTANT>
  <XDFTABLE uniqueid="0x2">
    <title>F4</title>
    <XDFAXIS id="x">
      <EMBEDDEDDATA mmedelementsizebits="16" mmedmajorstridebits="-32" />
      <indexcount>4</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="y">
      <indexcount>1</indexcount>
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
    <XDFAXIS id="z">
      <EMBEDDEDDATA mmedtypeflags="0x10002" mmedelementsizebits="32" mmedrowcount="1" mmedcolcount="4" />
      <MATH equation="X"><VAR id="X" /></MATH>
    </XDFAXIS>
  </XDFTABLE>
</XDFFORMAT>
`
	m, _, _, _, err := view([]byte(xdf))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 2 {
		t.Fatalf("objects: %d", len(m.Objects))
	}
	for _, o := range m.Objects {
		if o.Address != 0 || !o.Data.Float {
			t.Fatalf("%s: addr=%#x data=%+v", o.ID, o.Address, o.Data)
		}
	}
}

func TestWriteWiderCellsAlwaysTypeFlags(t *testing.T) {
	o := &model.Object{
		ID: "V", Shape: "value", Rows: 1, Cols: 1, Address: 0x10,
		Data:  model.Data{Bits: 16}, // endian unset → little
		Value: model.Value{Conversion: model.Conversion{Factor: 1}},
	}
	b, err := Write(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{o}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`mmedtypeflags="0x02"`)) {
		t.Fatalf("expected LE flag in %s", b)
	}
	o.Data.Endian = "big"
	b, err = Write(&model.Model{Schema: model.SchemaID, Objects: []*model.Object{o}}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`mmedtypeflags="0x00"`)) {
		t.Fatalf("expected explicit big flag in %s", b)
	}
}
