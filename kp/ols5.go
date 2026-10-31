package kp

import "math"

// OLS5 is the WinOLS 5 pack layout (header 0x255). Read-only for now.

type ols5MapExtra struct {
	CommentPre  Hex // 12 bytes after CommentFlag (unconfirmed)
	MarkedExtra Hex // 69 bytes when Marked != 0 (unconfirmed)
	NameExtra   [2]int32
	IDPad4      [4]int32 // replaces IDPad/IDFlag/IDState (unconfirmed)
	RangePad    [12]int32
}

type ols5AxisExtra struct {
	ValueMid [3]int32 // between axis description and units (unconfirmed)
	ID       string   // after SignatureByte, no NUL (unconfirmed)
	IDPad    [2]int32
}

// Side maps keep OLS5 residue without widening the exported Map/Axis JSON yet.
var ols5Maps = map[*Map]*ols5MapExtra{}
var ols5Axes = map[*Axis]*ols5AxisExtra{}

func (m *Map) ols5() *ols5MapExtra {
	if e := ols5Maps[m]; e != nil {
		return e
	}
	e := &ols5MapExtra{}
	ols5Maps[m] = e
	return e
}

func (a *Axis) ols5() *ols5AxisExtra {
	if e := ols5Axes[a]; e != nil {
		return e
	}
	e := &ols5AxisExtra{}
	ols5Axes[a] = e
	return e
}

func (m *Map) codeOLS5(c *codec) {
	if c.enc {
		c.fail("WinOLS 5 map encode is not supported yet")
	}
	ex := m.ols5()
	c.u8(&m.Marked)
	c.i32(&m.LinkID)
	c.str(&m.Comment)
	c.u8(&m.CommentFlag)
	c.raw(&ex.CommentPre, 12)
	if m.Marked != 0 {
		c.raw(&ex.MarkedExtra, 69)
	}
	c.strOLS5Name(&m.Name)
	c.i32(&ex.NameExtra[0])
	c.i32(&ex.NameExtra[1])
	c.i32((*int32)(&m.Organization))
	c.i32(&m.RightPane)
	c.i32((*int32)(&m.Type))
	c.i32(&m.Width)
	c.i32(&m.Base)
	c.i32(&m.FolderID)
	c.str(&m.ID)
	for i := range ex.IDPad4 {
		c.i32(&ex.IDPad4[i])
	}
	for i := range m.Range {
		for j := range m.Range[i] {
			var f float64
			c.f64(&f)
			m.Range[i][j] = int64(math.Round(f))
		}
	}
	for i := range ex.RangePad {
		c.i32(&ex.RangePad[i])
	}
	c.bool(&m.Reciprocal)
	c.bool(&m.Signed)
	c.bool(&m.Difference)
	c.bool(&m.Percent)
	c.i32(&m.Cols)
	c.i32(&m.Rows)
	c.i32(&m.Cursor[0])
	c.i32(&m.Cursor[1])
	c.i32(&m.Precision)
	c.valueOLS5Map(&m.Value)
	c.u32(&m.Start)
	c.u32(&m.End)
	c.i32(&m.ImageSize)
	c.i32(&m.ImageSizePad[0])
	c.i32(&m.ImageSizePad[1])
	c.u32(&m.Addr2)
	c.i32(&m.Addr2Flags[0])
	c.i32(&m.Addr2Flags[1])
	c.i32(&m.Addr2ImageSize)
	c.i32(&m.Addr2Pad)
	m.X = new(Axis)
	m.Y = new(Axis)
	m.X.codeOLS5(c)
	m.Y.codeOLS5(c)
	c.i32(&m.YFlag)
	c.i16(&m.YPad)
	c.i32(&m.StoredCols)
	c.i32(&m.StoredRows)
	c.i32(&m.ViewMode)
	c.u8(&m.ViewFlags[0])
	c.u8(&m.ViewFlags[1])
	c.f64(&m.View2DScale[0])
	c.f64(&m.View2DScale[1])
	c.i32(&m.View2DRef)
	c.u8(&m.ViewFlag)
	c.f64(&m.View3DRotation[0])
	c.f64(&m.View3DRotation[1])
	c.f64(&m.ViewScale)
	c.i32(&m.ViewScaleRef)
	c.f64(&m.ViewOffset)
	c.raw(&m.Term2, 2)
}

func (a *Axis) codeOLS5(c *codec) {
	if c.enc {
		c.fail("WinOLS 5 axis encode is not supported yet")
	}
	ex := a.ols5()
	c.strRaw(&a.Value.Description)
	for i := range ex.ValueMid {
		c.i32(&ex.ValueMid[i])
	}
	c.strRaw(&a.Value.Units)
	c.f64(&a.Value.Factor)
	c.f64(&a.Value.Offset)
	c.i32((*int32)(&a.DataSource))
	c.u32(&a.Addr)
	c.i32((*int32)(&a.Type))
	c.i32(&a.Width)
	c.i32(&a.Base)
	c.mirrorI32(&a.Mirror)
	c.raw(&a.MirrorPad, 9)
	c.bool(&a.Reciprocal)
	c.i32(&a.Precision)
	c.bool(&a.Signed)
	c.hexBytes(&a.Values, 4)
	c.i32(&a.DataHeader)
	c.i32(&a.SignatureByte)
	c.strRaw(&ex.ID)
	c.i32(&ex.IDPad[0])
	c.i32(&ex.IDPad[1])
}

func (c *codec) valueOLS5Map(v *Value) {
	c.strRaw(&v.Description)
	c.strRaw(&v.Units)
	c.f64(&v.Factor)
	c.f64(&v.Offset)
}

func (c *codec) strOLS5Name(p *string) {
	var n int32
	c.i32(&n)
	if n <= 0 {
		c.take(4)
		*p = ""
		return
	}
	if int(n) > len(c.b)-c.pos {
		c.pos -= 4
		c.fail("bad name length %d", n)
	}
	*p = latin9(c.take(int(n)))
	if c.take(1)[0] != 0 {
		c.pos--
		c.fail("name not NUL terminated")
	}
	c.take(3)
}
