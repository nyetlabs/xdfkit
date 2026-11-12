package kp

// Map is one map definition. Fields are in file order (see codec for the tags).
// Names follow WinOLS HelpEn where known (docs/kp-format.md).
type Map struct {
	Index          int         `json:"index" kp:"-"`
	Key            string      `json:"-" kp:"-"`                // model object key when the map came from model JSON; lint only
	Marked         byte        `json:"marked"`                  // 0 or 1: WinOLS map-list marker (Insert/delete marker; Only marked / Only unmarked)
	LinkID         int32       `json:"linkId,omitzero" kp:"v2"` // -1: id of a linked map, -1 for none (unconfirmed)
	Comment        string      `json:"comment,omitzero" kp:"v2"`
	CommentFlag    byte        `json:"commentFlag,omitzero" kp:"v2"` // 0 (unconfirmed)
	Name           string      `json:"name"`
	Type           Org         `json:"type"`    // HelpEn Type / Typ (eEinzel…)
	RWin           int32       `json:"rwin"`    // HelpEn RWin: 0 none, 1 hex, 2 bars, 3 both (CSV-confirmed on 8xx)
	DataOrg        DataOrg     `json:"dataOrg"` // HelpEn DataOrg (eByte, eHiLo, …)
	Width          int32       `json:"width"`   // element size in bytes; equals DataOrg's width in every known pack
	Radix          int32       `json:"radix"`   // HelpEn Radix (10/16/2)
	FolderID       int32       `json:"folderId"`
	ID             string      `json:"id"`
	IDPad          int32       `json:"idPad"`                           // 0 (unconfirmed)
	IDFlag         byte        `json:"idFlag"`                          // 0 (unconfirmed)
	IDState        int32       `json:"idState,omitzero" kp:"v2,hybrid"` // 0 or 1, mostly per pack (unconfirmed); was wrongly named Marked
	Range          [3][2]int64 `json:"range"`                           // value range low/high for 8-, 16- and 32-bit (32-bit unconfirmed)
	Reciprocal     bool        `json:"reciprocal"`                      // HelpEn bKehrwert / bReciprocal
	Signed         bool        `json:"signed"`                          // HelpEn bSigned / bVorzeichen (CSV-confirmed on 8xx)
	Difference     bool        `json:"difference"`                      // HelpEn bDelta candidate (unconfirmed; always 0 on 839 CSV pair)
	Percent        bool        `json:"percent"`                         // HelpEn bPercent / bProzent candidate (unconfirmed; always 0 on 839 CSV pair)
	Cols           int32       `json:"cols"`
	Rows           int32       `json:"rows"`
	Cursor         [2]int32    `json:"cursor"`    // editor state, the last cursor cell (unconfirmed)
	Precision      int32       `json:"precision"` // HelpEn Nachkommastellen / Precision
	Value          Value       `json:"value"`
	Start          uint32      `json:"start"`
	End            uint32      `json:"end"`
	ImageSize      int32       `json:"imageSize"`                     // matches the image length in every known pack
	ImageSizePad   [2]int32    `json:"imageSizePad,omitzero" kp:"v2"` // 0 (unconfirmed)
	Addr2          uint32      `json:"addr2"`                         // location as of the last properties edit (unconfirmed)
	Addr2Flags     [2]int32    `json:"addr2Flags"`                    // first -1 or 0 per pack, second 0 (unconfirmed)
	Addr2ImageSize int32       `json:"addr2ImageSize"`                // the image length, or 0 (unconfirmed)
	Addr2Pad       int32       `json:"addr2Pad"`                      // 0 (unconfirmed)
	X              *Axis       `json:"x"`
	Y              *Axis       `json:"y"`
	YFlag          int32       `json:"yFlag"`      // 0, 1 on two maps (unconfirmed)
	YPad           int16       `json:"yPad"`       // 0 (unconfirmed)
	StoredCols     int32       `json:"storedCols"` // Cols and Rows in storage order: swapped on 2d-inverse maps
	StoredRows     int32       `json:"storedRows"`
	ViewMode       int32       `json:"viewMode"`         // HelpEn ViewMode: 1 text, 2 2D, 3 3D (CSV-confirmed on 8xx)
	ViewFlags      [2]byte     `json:"viewFlags"`        // 1, 1 (unconfirmed)
	View2DScale    [2]float64  `json:"view2DScale"`      // 2D view scaling, 1.0, 1.0 by default (unconfirmed)
	View2DRef      int32       `json:"view2DRef"`        // -1 until the view mode is first changed, then 0 (unconfirmed meaning)
	ViewFlag       byte        `json:"viewFlag"`         // 1, 0 on a few maps (unconfirmed)
	View3DRotation [2]float64  `json:"view3DRotation"`   // 3D view angle and zoom, 300.0, 1.0 by default (unconfirmed)
	ViewScale      float64     `json:"viewScale"`        // the 3D view's auto scale, 0 until first shown in 3D (unconfirmed)
	ViewScaleRef   int32       `json:"viewScaleRef"`     // -1 exactly where ViewScale is 0 (unconfirmed)
	ViewOffset     float64     `json:"viewOffset"`       // view state, with ViewScale (unconfirmed)
	Term2          Hex         `json:"term2" kp:"len=3"` // first two bytes are always 1
}

func (m *Map) check(c *codec) {
	if len(m.Term2) < 2 || m.Term2[0] != 1 || m.Term2[1] != 1 {
		c.pos -= len(m.Term2)
		c.fail("unexpected term2 %v", m.Term2)
	}
	twoD := m.Type == OrgTwoD || m.Type == OrgTwoDInv
	m.X.Defined = twoD || m.Type == OrgOneD
	m.Y.Defined = twoD
}

// Value is the scaling block shared by maps and axes (Feldwerte / Stuetz).
type Value struct {
	Description string  `json:"description"`
	Units       string  `json:"units"`
	Factor      float64 `json:"factor"`
	Offset      float64 `json:"offset"`
}

// Axis is an X or Y axis record. Every map stores both; Defined is set on the axes
// its Type implies (X for 1D, X and Y for 2D). The unused slots (the second
// axis of a 1D map, both axes of a single value) are still parsed.
type Axis struct {
	Defined       bool       `json:"defined" kp:"-"`
	Value         Value      `json:"value"`
	DataSource    DataSource `json:"dataSource"` // HelpEn DataSrc
	Addr          uint32     `json:"addr"`
	DataOrg       DataOrg    `json:"dataOrg"`                             // HelpEn DataOrg
	Width         int32      `json:"width"`                               // element size in bytes; equals DataOrg's width on all but 2 known axes
	Radix         int32      `json:"radix"`                               // HelpEn Radix
	Backwards     bool       `json:"backwards" kp:"hook"`                   // HelpEn bRueckwaerts / bBackwards: show descending; v2: byte 0 of the axis flag word, v1: byte
	FlagPad       Hex        `json:"flagPad,omitzero" kp:"v2only,len=2"`    // high bytes of the axis flag word
	BackwardsPad  Hex        `json:"backwardsPad,omitzero" kp:"v2,len=9"`   // zeros (unconfirmed); V2 hybrid: 1 byte via Backwards hook
	UnknownFlag   byte       `json:"unknownFlag,omitzero" kp:"v2only"`      // byte where the docs place Reciprocal
	Reciprocal    bool       `json:"reciprocal" kp:"v1,ols5"`               // bKehrwert; v2: byte 1 of the axis flag word (see hook)
	Precision     int32      `json:"precision"`                           // -1 on some unused slots
	Signed        bool       `json:"signed"`
	Values        Hex        `json:"values" kp:"bytes"` // free editable values in the axis type, 4 bytes per point
	DataHeader    int32      `json:"dataHeader"`        // HelpEn DataHeader (unconfirmed)
	SignatureByte int32      `json:"signatureByte"`     // HelpEn SignaturByte / SignatureByte, -1 for none (unconfirmed)
}

// hook codes Backwards: v1 byte; v2/OLS5 byte 0 of the axis flag word, whose
// byte 1 is Reciprocal (WinOLS save: ticking Reciprocal writes 00 01 00 00);
// V2 hybrid int32 plus 1-byte BackwardsPad.
func (a *Axis) hook(c *codec, _ string) {
	if c.hybrid && c.block {
		c.mirrorI32(&a.Backwards)
		c.raw(&a.BackwardsPad, 1)
		return
	}
	if c.layout == V1 {
		c.bool(&a.Backwards)
		return
	}
	b, r := byte(0), byte(0)
	if a.Backwards {
		b = 1
	}
	if a.Reciprocal {
		r = 1
	}
	c.u8(&b)
	c.u8(&r)
	if b > 1 || r > 1 {
		c.fail("axis flag word byte %d/%d", b, r)
	}
	a.Backwards, a.Reciprocal = b == 1, r == 1
}
