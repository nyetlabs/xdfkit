package kp

// Map is one map definition. Fields are in file order (see codec for the tags).
// Undecoded fields are named as described in the package doc.
type Map struct {
	Index          int         `json:"index" kp:"-"`
	Selected       byte        `json:"selected"`                // selected in the map list (unconfirmed): changes between exports of the same project
	LinkID         int32       `json:"linkId,omitzero" kp:"v2"` // -1: id of a linked map, -1 for none (unconfirmed)
	Comment        string      `json:"comment,omitzero" kp:"v2"`
	CommentFlag    byte        `json:"commentFlag,omitzero" kp:"v2"` // 0 (unconfirmed)
	Name           string      `json:"name"`
	Organization   Org         `json:"organization"`
	RightPane      int32       `json:"rightPane"` // the script's RWin: 0 none, 1 hex, 2 bars, 3 both (unconfirmed)
	Type           Type        `json:"type"`
	Width          int32       `json:"width"` // element size in bytes; equals Type's width in every known pack
	Base           int32       `json:"base"`
	FolderID       int32       `json:"folderId"`
	ID             string      `json:"id"`
	IDPad          int32       `json:"idPad"`                   // 0 (unconfirmed)
	IDFlag         byte        `json:"idFlag"`                  // 0 (unconfirmed)
	Marked         int32       `json:"marked,omitzero" kp:"v2"` // 0 or 1, mostly per pack (unconfirmed)
	Range          [3][2]int64 `json:"range"`                   // map properties value range, low and high, for 8-, 16- and 32-bit data (32-bit unconfirmed)
	Reciprocal     bool        `json:"reciprocal"`              // the script's bKehrwert (unconfirmed)
	Signed         bool        `json:"signed"`                  // bVorzeichen (unconfirmed)
	Difference     bool        `json:"difference"`              // bDelta: map window "difference" toggle (unconfirmed)
	Percent        bool        `json:"percent"`                 // bProzent: map window "percent" toggle (unconfirmed)
	Cols           int32       `json:"cols"`
	Rows           int32       `json:"rows"`
	Cursor         [2]int32    `json:"cursor"` // editor state, the last cursor cell (unconfirmed)
	Precision      int32       `json:"precision"`
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
	ViewMode       int32       `json:"viewMode"`         // 1 text, 2 2D, 3 3D (the script's ViewMode)
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
	if m.Term2[0] != 1 || m.Term2[1] != 1 {
		c.pos -= 3
		c.fail("unexpected term2 %v", m.Term2)
	}
	twoD := m.Organization == OrgTwoD || m.Organization == OrgTwoDInv
	m.X.Defined = twoD || m.Organization == OrgOneD
	m.Y.Defined = twoD
}

// Value is the scaling block shared by maps and axes.
type Value struct {
	Description string  `json:"description"`
	Units       string  `json:"units"`
	Factor      float64 `json:"factor"`
	Offset      float64 `json:"offset"`
}

// Axis is an X or Y axis record. Every map stores both; Defined is set on the axes
// its organisation implies (X for 1D, X and Y for 2D). The unused slots (the second
// axis of a 1D map, both axes of a single value) are still parsed.
type Axis struct {
	Defined       bool       `json:"defined" kp:"-"`
	Value         Value      `json:"value"`
	DataSource    DataSource `json:"dataSource"`
	Addr          uint32     `json:"addr"`
	Type          Type       `json:"type"`
	Width         int32      `json:"width"` // element size in bytes; equals Type's width on all but 2 known axes
	Base          int32      `json:"base"`
	Mirror        bool       `json:"mirror" kp:"hook"`                 // "mirror map": shown in descending order; v2: int32, v1: byte
	MirrorPad     Hex        `json:"mirrorPad,omitzero" kp:"v2,len=9"` // zeros (unconfirmed)
	Reciprocal    bool       `json:"reciprocal"`
	Precision     int32      `json:"precision"` // -1 on some unused slots
	Signed        bool       `json:"signed"`
	Values        Hex        `json:"values" kp:"bytes"` // free editable values in the axis type, 4 bytes per point
	DataHeader    int32      `json:"dataHeader"`        // the script's DataHeader: header bytes before the axis data (unconfirmed)
	SignatureByte int32      `json:"signatureByte"`     // the script's SignaturByte (marker byte before the axis), -1 for none (unconfirmed)
}

// hook codes Mirror: an int32 in v2, a byte in v1, 0 or 1 in both.
func (a *Axis) hook(c *codec, _ string) {
	if c.layout == V1 {
		c.bool(&a.Mirror)
		return
	}
	var v int32
	if a.Mirror {
		v = 1
	}
	c.i32(&v)
	if v != 0 && v != 1 {
		c.pos -= 4
		c.fail("mirror int32 %d", v)
	}
	a.Mirror = v == 1
}
