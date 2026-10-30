package a2l

import (
	"strings"
	"testing"

	"go.nyet.org/xdfkit/model"
)

const sample = `
ASAP2_VERSION 1 31
/begin PROJECT DEMO ""
  /begin HEADER ""
    VERSION "1.2"
  /end HEADER
  /begin MODULE MOD ""
    /begin MOD_COMMON ""
      BYTE_ORDER MSB_LAST
      ALIGNMENT_BYTE 1
      ALIGNMENT_WORD 2
      ALIGNMENT_LONG 2
    /end MOD_COMMON
    /begin MEMORY_SEGMENT SEG ""
      DATA FLASH INTERN
      0x810000
      0x1000
      -1 -1 -1 -1 -1
    /end MEMORY_SEGMENT
    /begin COMPU_METHOD ident ""
      RAT_FUNC "%3.0" ""
      COEFFS 0 1 0 0 0 1
    /end COMPU_METHOD
    /begin COMPU_METHOD recip ""
      RAT_FUNC "%6.1" "1/s"
      COEFFS 0 0 2560 0 1 0
    /end COMPU_METHOD
    /begin COMPU_METHOD odd ""
      RAT_FUNC "%3.0" ""
      COEFFS 1 0 0 0 0 1
    /end COMPU_METHOD
    /begin RECORD_LAYOUT Kf
      NO_AXIS_PTS_X 1 UBYTE
      AXIS_PTS_X 2 UBYTE
      FNC_VALUES 3 UWORD COLUMN_DIR DIRECT
    /end RECORD_LAYOUT
    /begin RECORD_LAYOUT Scalar
      FNC_VALUES 1 UBYTE
    /end RECORD_LAYOUT
    /begin RECORD_LAYOUT AxisLay
      NO_AXIS_PTS_X 1 UBYTE
      AXIS_PTS_X 2 UWORD INDEX_INCR DIRECT
    /end RECORD_LAYOUT
    /begin AXIS_PTS SST "speed"
      0x800100
      NO_INPUT
      AxisLay
      0
      ident
      4
      0
      100
    /end AXIS_PTS
    /begin CHARACTERISTIC Curve "a curve"
      CURVE
      0x800200
      Kf
      0
      recip
      0
      10
      /begin AXIS_DESCR STD_AXIS
        NO_INPUT
        ident
        4
        0
        10
      /end AXIS_DESCR
    /end CHARACTERISTIC
    /begin CHARACTERISTIC Map "a map"
      MAP
      0x800300
      Scalar
      0
      ident
      0
      1
      /begin AXIS_DESCR COM_AXIS
        NO_INPUT
        ident
        4
        0
        100
        AXIS_PTS_REF SST
      /end AXIS_DESCR
      /begin AXIS_DESCR FIX_AXIS
        NO_INPUT
        ident
        3
        0
        2
        FIX_AXIS_PAR 0 0 3
      /end AXIS_DESCR
    /end CHARACTERISTIC
    /begin CHARACTERISTIC Block "bytes"
      VAL_BLK
      0x810020
      Scalar
      0
      ident
      0
      1
      NUMBER 8
    /end CHARACTERISTIC
    /begin CHARACTERISTIC Odd "odd scale"
      VALUE
      0x800500
      Scalar
      0
      odd
      0
      1
    /end CHARACTERISTIC
    /begin CHARACTERISTIC Cube "no"
      CUBOID
      0x800400
      Scalar
      0
      ident
      0
      1
    /end CHARACTERISTIC
    /begin MEASUREMENT M ""
      UBYTE ident 1 0 0 255
      ECU_ADDRESS 0x800000
    /end MEASUREMENT
    /begin FUNCTION F1 "group"
      /begin DEF_CHARACTERISTIC
        Curve
      /end DEF_CHARACTERISTIC
    /end FUNCTION
  /end MODULE
/end PROJECT
`

func TestRead(t *testing.T) {
	m, warnings, err := Read([]byte(sample), "demo.a2l")
	if err != nil {
		t.Fatal(err)
	}
	if m.Provenance.Format != "a2l" || m.Provenance.Origin != "a2l" || m.Provenance.File != "demo.a2l" {
		t.Fatalf("provenance %+v", m.Provenance)
	}
	if m.Project.Name != "DEMO" || m.Project.Version != "1.2" {
		t.Fatalf("project %+v", m.Project)
	}
	byID := map[string]*model.Object{}
	for _, o := range m.Objects {
		byID[o.ID] = o
	}
	c := byID["Curve"]
	if c == nil || c.Shape != "1d" || c.Cols != 4 || c.Address != 0x206 {
		t.Fatalf("curve %+v", c)
	}
	if c.Data.Bits != 16 || c.Data.Endian != "little" || !c.Value.Conversion.Reciprocal || c.Value.Conversion.Factor != 2560 || c.Value.Precision != 1 || c.Value.Units != "1/s" {
		t.Fatalf("curve value %+v data %+v", c.Value, c.Data)
	}
	if c.X == nil || c.X.Source != "image" || c.X.Address == nil || *c.X.Address != 0x201 || c.X.Data.Bits != 8 {
		t.Fatalf("curve axis %+v", c.X)
	}
	if len(c.Categories) != 1 || m.Categories[c.Categories[0]].Name != "F1" {
		t.Fatalf("categories %+v obj %v", m.Categories, c.Categories)
	}
	mp := byID["Map"]
	if mp == nil || mp.Shape != "2d" || mp.Rows != 3 || mp.Cols != 4 || mp.Address != 0x300 {
		t.Fatalf("map %+v", mp)
	}
	if mp.X == nil || mp.X.Address == nil || *mp.X.Address != 0x102 || mp.Y == nil || mp.Y.Source != "ordinal" {
		t.Fatalf("map axes x %+v y %+v", mp.X, mp.Y)
	}
	sst := byID["SST"]
	if sst == nil || sst.Shape != "1d" || sst.Address != 0x102 || sst.Cols != 4 || sst.Data.Bits != 16 {
		t.Fatalf("axis pts %+v", sst)
	}
	blk := byID["Block"]
	if blk == nil || blk.Shape != "1d" || blk.Cols != 8 || blk.Address != 0x10020 || blk.X == nil || blk.X.Source != "ordinal" {
		t.Fatalf("valblk %+v", blk)
	}
	if _, ok := byID["Cube"]; ok {
		t.Fatal("cuboid was kept")
	}
	odd := false
	meas := false
	for _, w := range warnings {
		if strings.Contains(w, "not linear") {
			odd = true
		}
		if strings.Contains(w, "MEASUREMENT") {
			meas = true
		}
	}
	if !odd || !meas {
		t.Fatalf("warnings %v", warnings)
	}
}

func TestNotA2L(t *testing.T) {
	if _, _, err := Read([]byte("not a file\n"), "x"); err == nil {
		t.Fatal("expected an error")
	}
}
