package model

import "testing"

func TestScanExtremes(t *testing.T) {
	img := []byte{0x10, 0x20, 0x05, 0xff}
	o := &Object{
		Key: "t", Address: 0, Rows: 1, Cols: 4, Data: Data{Bits: 8},
		Value: Value{Conversion: Conversion{Factor: 0.5}},
	}
	e, err := ScanExtremes(o, img)
	if err != nil || e == nil || e.Min != 2.5 || e.Max != 127.5 || e.RawMin != "0x5" || e.RawMax != "0xff" {
		t.Fatalf("%v %+v", err, e)
	}
	o.Value.Conversion.Factor = -1
	e, err = ScanExtremes(o, img)
	if err != nil || e.Min != -255 || e.Max != -5 || e.RawMin != "0xff" || e.RawMax != "0x5" {
		t.Fatalf("neg %v %+v", err, e)
	}
	o.Rows, o.Cols = 0, 0
	if e, err = ScanExtremes(o, img); err != nil || e != nil {
		t.Fatalf("empty %v %+v", err, e)
	}
	m := &Model{Objects: []*Object{{
		Key: "a", Address: 0, Rows: 1, Cols: 2, Data: Data{Bits: 8},
		Value: Value{Conversion: Conversion{Factor: 1}},
	}}}
	if err := ApplyScan(m, img[:2]); err != nil {
		t.Fatal(err)
	}
	e = m.Objects[0].Extremes
	if e == nil || e.Min != 16 || e.Max != 32 || e.RawMin != "0x10" || e.RawMax != "0x20" {
		t.Fatalf("ApplyScan %+v", e)
	}
}
