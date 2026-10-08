package kp

// Map is one map definition. Fields are in file order (see codec for the tags).
// Unk* fields are undecoded and keep mapdump's hN names; each should get a real
// name once its meaning is known.
type Map struct {
	Index        int      `json:"index" kp:"-"`
	UnkH0        byte     `json:"unkH0"`
	UnkH0a       int32    `json:"unkH0a,omitzero" kp:"v2"`
	Comment      string   `json:"comment,omitzero" kp:"v2"`
	UnkH0b       byte     `json:"unkH0b,omitzero" kp:"v2"`
	Name         string   `json:"name"`
	Organization Org      `json:"organization"`
	UnkH         int32    `json:"unkH"`
	Type         Type     `json:"type"`
	UnkHa        int32    `json:"unkHa"`
	Base         int32    `json:"base"`
	FolderID     int32    `json:"folderId"`
	ID           string   `json:"id"`
	UnkH1        int32    `json:"unkH1"`
	UnkH1a       byte     `json:"unkH1a"`
	UnkH1b       int32    `json:"unkH1b,omitzero" kp:"v2"`
	Range        [4]int32 `json:"range"` // meaning unconfirmed
	UnkH2        [8]int32 `json:"unkH2"`
	Reciprocal   bool     `json:"reciprocal"`
	Signed       bool     `json:"signed"`
	Difference   bool     `json:"difference"`
	Percent      bool     `json:"percent"`
	Cols         int32    `json:"cols"`
	Rows         int32    `json:"rows"`
	UnkH3        [2]int32 `json:"unkH3"`
	Precision    int32    `json:"precision"`
	Value        Value    `json:"value"`
	Start        uint32   `json:"start"`
	End          uint32   `json:"end"`
	UnkH4        int32    `json:"unkH4"`
	UnkH4a       [2]int32 `json:"unkH4a,omitzero" kp:"v2"`
	Addr2        uint32   `json:"addr2"` // meaning unconfirmed
	UnkH5        [2]int32 `json:"unkH5"`
	UnkH6        int32    `json:"unkH6"`
	UnkH7        int32    `json:"unkH7"`
	X            *Axis    `json:"x"`
	Y            *Axis    `json:"y"`
	UnkH8        int32    `json:"unkH8"`
	UnkH8a       int16    `json:"unkH8a"`
	UnkH9        [5]int32 `json:"unkH9"`
	UnkH9a       [7]int16 `json:"unkH9a"`
	UnkH9b       int32    `json:"unkH9b"`
	UnkH9c       byte     `json:"unkH9c"`
	UnkH10       [6]int32 `json:"unkH10"` // holds doubles such as 300.0, 1.0, -0.9
	UnkH11       [2]int32 `json:"unkH11"`
	Term2        Hex      `json:"term2" kp:"len=3"` // first two bytes are always 1
}

func (m *Map) check(c *codec) {
	if m.Term2[0] != 1 || m.Term2[1] != 1 {
		c.pos -= 3
		c.fail("unexpected term2 %v", m.Term2)
	}
}

// Value is the scaling block shared by maps and axes.
type Value struct {
	Description string  `json:"description"`
	Units       string  `json:"units"`
	Factor      float64 `json:"factor"`
	Offset      float64 `json:"offset"`
}

// Axis is an X or Y axis record. Undefined slots (the second axis of a 1D map, both
// axes of a single value) are still stored and parsed.
type Axis struct {
	Defined    bool       `json:"defined" kp:"-"`
	Value      Value      `json:"value"`
	DataSource DataSource `json:"dataSource"`
	Addr       uint32     `json:"addr"`
	Type       Type       `json:"type"`
	UnkH1      int32      `json:"unkH1"`
	Base       int32      `json:"base"`
	UnkH1a     [3]int32   `json:"unkH1a,omitzero" kp:"v2"` // [0] is 1 on undefined axes
	UnkH2      byte       `json:"unkH2"`                   // v1: 1 on undefined axes
	Reciprocal bool       `json:"reciprocal"`
	Precision  byte       `json:"precision"`
	UnkH3      Hex        `json:"unkH3" kp:"len=3"`
	Signed     bool       `json:"signed"`
	UnkH4      []int32    `json:"unkH4" kp:"bytes"`
	UnkH5      int32      `json:"unkH5"`
	Signature  int32      `json:"signature"` // meaning unconfirmed
}

func (a *Axis) check(c *codec) {
	if c.layout == V2 {
		a.Defined = a.UnkH1a[0] == 0
	} else {
		a.Defined = a.UnkH2 != 1
	}
}
