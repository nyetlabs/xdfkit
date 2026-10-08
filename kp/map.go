package kp

// Map is one map definition. Fields are in file order (see codec for the tags).
// Undecoded fields are named as described in the package doc.
type Map struct {
	Index             int        `json:"index" kp:"-"`
	Unk00             byte       `json:"unk00"`
	Unk01             int32      `json:"unk01,omitzero" kp:"v2"`
	Comment           string     `json:"comment,omitzero" kp:"v2"`
	CommentUnk00      byte       `json:"commentUnk00,omitzero" kp:"v2"`
	Name              string     `json:"name"`
	Organization      Org        `json:"organization"`
	OrganizationUnk00 int32      `json:"organizationUnk00"`
	Type              Type       `json:"type"`
	Width             int32      `json:"width"` // element size in bytes; equals Type's width in every known pack
	Base              int32      `json:"base"`
	FolderID          int32      `json:"folderId"`
	ID                string     `json:"id"`
	IDUnk00           int32      `json:"idUnk00"`
	IDUnk04           byte       `json:"idUnk04"`
	IDUnk05           int32      `json:"idUnk05,omitzero" kp:"v2"`
	Range             [4]int32   `json:"range"` // meaning unconfirmed
	RangeUnk00        [8]int32   `json:"rangeUnk00"`
	Reciprocal        bool       `json:"reciprocal"`
	Signed            bool       `json:"signed"`
	Difference        bool       `json:"difference"`
	Percent           bool       `json:"percent"`
	Cols              int32      `json:"cols"`
	Rows              int32      `json:"rows"`
	RowsUnk00         [2]int32   `json:"rowsUnk00"` // probably editor state (docs/kp-format.md)
	Precision         int32      `json:"precision"`
	Value             Value      `json:"value"`
	Start             uint32     `json:"start"`
	End               uint32     `json:"end"`
	ImageSize         int32      `json:"imageSize"` // matches the image length in every known pack
	ImageSizeUnk00    [2]int32   `json:"imageSizeUnk00,omitzero" kp:"v2"`
	Addr2             uint32     `json:"addr2"` // meaning unconfirmed
	Addr2Unk00        [2]int32   `json:"addr2Unk00"`
	Addr2Unk08        int32      `json:"addr2Unk08"`
	Addr2Unk0C        int32      `json:"addr2Unk0C"`
	X                 *Axis      `json:"x"`
	Y                 *Axis      `json:"y"`
	YUnk00            int32      `json:"yUnk00"`
	YUnk04            int16      `json:"yUnk04"`
	YUnk06            int32      `json:"yUnk06"` // with YUnk0A, the dimensions in an order that varies (docs/kp-format.md)
	YUnk0A            int32      `json:"yUnk0A"`
	YUnk0E            int32      `json:"yUnk0E"`
	YUnk12            [2]byte    `json:"yUnk12"`
	YUnk14            [2]float64 `json:"yUnk14"`
	YUnk24            int32      `json:"yUnk24"`
	YUnk28            byte       `json:"yUnk28"`
	YUnk29            [2]float64 `json:"yUnk29"`
	YUnk39            float64    `json:"yUnk39"`
	YUnk41            int32      `json:"yUnk41"`
	YUnk45            float64    `json:"yUnk45"`
	Term2             Hex        `json:"term2" kp:"len=3"` // first two bytes are always 1
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
	Defined        bool       `json:"defined" kp:"-"`
	Value          Value      `json:"value"`
	DataSource     DataSource `json:"dataSource"`
	Addr           uint32     `json:"addr"`
	Type           Type       `json:"type"`
	Width          int32      `json:"width"` // element size in bytes; equals Type's width on all but 2 known axes
	Base           int32      `json:"base"`
	Mirror         bool       `json:"mirror" kp:"hook"` // "mirror map": shown in descending order; v2: int32, v1: byte
	MirrorUnk00    Hex        `json:"mirrorUnk00,omitzero" kp:"v2,len=9"`
	Reciprocal     bool       `json:"reciprocal"`
	Precision      byte       `json:"precision"`
	PrecisionUnk00 Hex        `json:"precisionUnk00" kp:"len=3"`
	Signed         bool       `json:"signed"`
	SignedUnk00    []int32    `json:"signedUnk00" kp:"bytes"`
	DataHeader     int32      `json:"dataHeader"`    // likely the script's DataHeader: header bytes before the axis data
	SignatureByte  int32      `json:"signatureByte"` // likely the script's SignaturByte (marker byte before the axis); -1 when none
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
