package kp

import (
	"encoding/binary"
	"math"
)

// OLS5 is the record walk for 5xx and 8xx. Read-only.
// Trailer and MarkedExtra lengths come from ols5SizesFor (50x vs other 5xx; 8xx scans).

type ols5MapExtra struct {
	CommentPre  Hex // 12 bytes after CommentFlag (unconfirmed)
	MarkedExtra Hex // length from ols5SizesFor (0 = scan), when Marked != 0
	NameExtra   [2]int32
	IDPad4      [4]int32 // replaces IDPad/IDFlag/IDState (unconfirmed)
	RangePad    [12]int32
}

type ols5AxisExtra struct {
	ValueMid   [3]int32 // between axis description and units (unconfirmed)
	ID         string   // after SignatureByte, no NUL (5xx; unconfirmed)
	IDPad      [2]int32
	IDCode     [3]int32 // 8xx: replaces ID string + IDPad (unconfirmed; often -1,0,0 or 2,0,0)
	TailPad    Hex      // 8xx: 20 bytes after each axis (unconfirmed)
	SignedRaw  int32    // 8xx: raw Axis.Signed byte when structured; unconfirmed
	ValuesBlob Hex      // 8xx: opaque Signed-through-IDCode when Values layout drifts (unconfirmed)
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
		c.fail("OLS5 map encode is not supported yet")
	}
	ex := m.ols5()
	c.u8(&m.Marked)
	c.i32(&m.LinkID)
	c.str(&m.Comment)
	c.u8(&m.CommentFlag)
	c.raw(&ex.CommentPre, 12)
	if m.Marked != 0 {
		n := c.sizes.MarkedExtra
		if n == 0 {
			n = ols5MarkedExtra8xx(c)
		}
		c.raw(&ex.MarkedExtra, n)
		if c.sizes.MarkedExtra == 0 {
			// Long comments sit after MarkedExtra (empty Comment above stays empty).
			ols5CommentAfterMarked8xx(c, m)
		}
	}
	c.strOLS5Name(&m.Name)
	c.i32(&ex.NameExtra[0])
	c.i32(&ex.NameExtra[1])
	c.i32((*int32)(&m.Type))
	c.i32(&m.RWin)
	c.i32((*int32)(&m.DataOrg))
	c.i32(&m.Width)
	c.i32(&m.Radix)
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
		c.fail("OLS5 axis encode is not supported yet")
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
	c.i32((*int32)(&a.DataOrg))
	c.i32(&a.Width)
	c.i32(&a.Radix)
	c.mirrorI32(&a.Backwards)
	c.raw(&a.BackwardsPad, 9)
	c.bool(&a.Reciprocal)
	c.i32(&a.Precision)
	if is8xx(c.headerID) {
		c.code8xxAxisTail(a, ex)
		return
	}
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

// code8xxAxisTail reads Signed through TailPad for 8xx. The common form is a
// 1-byte Signed flag, Values/DataHeader/SignatureByte, a 3×int32 IDCode, then a
// 20-byte TailPad (0,0,-1,0,0). When that does not land on TailPad, the
// intervening bytes are kept opaque up to the next TailPad (DMDDLWS-class).
func (c *codec) code8xxAxisTail(a *Axis, ex *ols5AxisExtra) {
	start := c.pos
	if try8xxAxisTailStructured(c, a, ex) {
		c.raw(&ex.TailPad, 20)
		return
	}
	c.pos = start
	const window = 128
	// Structured layout needs 25 bytes to TailPad; reject shorter false hits.
	tailAt := find8xxAxisTailPad(c.b, c.pos+25, window)
	if tailAt < 0 {
		tailAt = find8xxAxisTailPad(c.b, c.pos+1, window)
	}
	if tailAt < 0 || tailAt-c.pos < 20 {
		c.fail("8xx axis TailPad not found")
	}
	var b byte
	c.u8(&b)
	ex.SignedRaw = int32(b)
	a.Signed = b != 0
	if c.pos < tailAt {
		c.raw(&ex.ValuesBlob, tailAt-c.pos)
	}
	c.raw(&ex.TailPad, 20)
}

func try8xxAxisTailStructured(c *codec, a *Axis, ex *ols5AxisExtra) bool {
	pos0 := c.pos
	var b byte
	c.u8(&b)
	if c.pos+4 > len(c.b) {
		c.pos = pos0
		return false
	}
	n := int(int32(binary.LittleEndian.Uint32(c.b[c.pos:])))
	if n < 0 || n > 1<<20 || (n%4 != 0 && n != 0) {
		c.pos = pos0
		return false
	}
	ex.SignedRaw = int32(b)
	a.Signed = b != 0
	c.hexBytes(&a.Values, 4)
	c.i32(&a.DataHeader)
	c.i32(&a.SignatureByte)
	for i := range ex.IDCode {
		c.i32(&ex.IDCode[i])
	}
	if !is8xxAxisTailPad(c.b, c.pos) {
		c.pos = pos0
		ex.SignedRaw = 0
		a.Signed = false
		a.Values = nil
		a.DataHeader = 0
		a.SignatureByte = 0
		ex.IDCode = [3]int32{}
		return false
	}
	return true
}

func is8xxAxisTailPad(b []byte, i int) bool {
	if i < 0 || i+20 > len(b) {
		return false
	}
	return int32(binary.LittleEndian.Uint32(b[i:])) == 0 &&
		int32(binary.LittleEndian.Uint32(b[i+4:])) == 0 &&
		int32(binary.LittleEndian.Uint32(b[i+8:])) == -1 &&
		int32(binary.LittleEndian.Uint32(b[i+12:])) == 0 &&
		int32(binary.LittleEndian.Uint32(b[i+16:])) == 0
}

// find8xxAxisTailPad finds the 20-byte TailPad (0,0,-1,0,0) at or after from.
func find8xxAxisTailPad(b []byte, from, window int) int {
	end := from + window
	if end > len(b)-20 {
		end = len(b) - 20
	}
	for i := from; i <= end; i++ {
		if is8xxAxisTailPad(b, i) {
			return i
		}
	}
	return -1
}

// ols5MarkedExtra8xx returns the marked-block size for 8xx by finding the
// following strOLS5Name (including the empty form: length <= 0 plus 4 bytes).
// Prefer 73. When a long comment sits between MarkedExtra and the name, land on
// the comment (ols5CommentAfterMarked8xx reads it next).
func ols5MarkedExtra8xx(c *codec) int {
	const prefer = 73
	if ols5NameAt(c.b, c.pos+prefer) {
		return prefer
	}
	if ols5CommentThenNameAt(c.b, c.pos+prefer) {
		return prefer
	}
	const max = 256
	for n := 8; n <= max; n++ {
		if ols5CommentThenNameAt(c.b, c.pos+n) {
			return n
		}
	}
	for n := 8; n <= max; n++ {
		// Empty OLS5 names (length <= 0) only at prefer; elsewhere require text.
		if ols5NonEmptyNameAt(c.b, c.pos+n) {
			return n
		}
	}
	c.fail("8xx marked block: name not found")
	return 0
}

func ols5NonEmptyNameAt(b []byte, i int) bool {
	if i < 0 || i+8 > len(b) {
		return false
	}
	nl := int(int32(binary.LittleEndian.Uint32(b[i:])))
	return nl > 0 && ols5NameAt(b, i)
}

// ols5CommentThenNameAt reports a long length-prefixed comment, optional zero
// pad, then an IdName (strOLS5Name).
func ols5CommentThenNameAt(b []byte, i int) bool {
	if i < 0 || i+8 > len(b) {
		return false
	}
	nl := int(int32(binary.LittleEndian.Uint32(b[i:])))
	if nl < 40 || nl > 2000 || i+4+nl > len(b) {
		return false
	}
	body := b[i+4 : i+4+nl]
	if body[0] < 'A' || (body[0] > 'Z' && body[0] < 'a') || body[0] > 'z' {
		return false
	}
	for _, ch := range body {
		if ch < 0x20 && ch != 0x09 && ch != 0x0a && ch != 0x0d {
			return false
		}
	}
	return ols5NameAfterPad(b, i+4+nl) >= 0
}

// ols5NameAfterPad returns the zero-pad length before a non-empty strOLS5Name, or -1.
func ols5NameAfterPad(b []byte, i int) int {
	for pad := 0; pad <= 16 && i+pad+8 <= len(b); pad++ {
		ok := true
		for j := 0; j < pad; j++ {
			if b[i+j] != 0 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		nl := int(int32(binary.LittleEndian.Uint32(b[i+pad:])))
		if nl > 0 && ols5NameAt(b, i+pad) {
			return pad
		}
	}
	return -1
}

// ols5CommentAfterMarked8xx reads an optional comment string after MarkedExtra.
func ols5CommentAfterMarked8xx(c *codec, m *Map) {
	if c.pos+4 > len(c.b) {
		return
	}
	nl := int(int32(binary.LittleEndian.Uint32(c.b[c.pos:])))
	if nl < 40 || nl > 2000 || c.pos+4+nl > len(c.b) {
		return
	}
	pad := ols5NameAfterPad(c.b, c.pos+4+nl)
	if pad < 0 {
		return
	}
	c.strRaw(&m.Comment) // no NUL; pad zeros (often 13) follow before Name
	if pad > 0 {
		c.take(pad)
	}
}

// ols5NameAt reports whether offset i begins a strOLS5Name value.
func ols5NameAt(b []byte, i int) bool {
	if i < 0 || i+8 > len(b) {
		return false
	}
	nl := int(int32(binary.LittleEndian.Uint32(b[i:])))
	if nl <= 0 {
		// empty: length word plus 4 pad bytes
		return true
	}
	if nl > 512 || i+4+nl+1+3 > len(b) {
		return false
	}
	if b[i+4+nl] != 0 {
		return false
	}
	pad := b[i+4+nl+1 : i+4+nl+1+3]
	if pad[0] != 0 || pad[1] != 0 || pad[2] != 0 {
		return false
	}
	for _, ch := range b[i+4 : i+4+nl] {
		if ch < 0x20 && ch != 0x09 {
			return false
		}
	}
	return true
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
