package kp

import (
	"encoding/json"
	"fmt"
)

// Org is the map organisation.
type Org int32

// Type is the element type (width, endianness, float).
type Type int32

// DataSource is where an axis takes its values from.
type DataSource int32

const (
	OrgSingle        Org        = 2
	OrgOneD          Org        = 3
	OrgTwoD          Org        = 4
	OrgTwoDInv       Org        = 5
	DSOrdinal        DataSource = 0
	DSEeprom         DataSource = 1
	DSEepromAdd      DataSource = 2
	DSEepromSubtract DataSource = 3
)

var orgNames = map[Org]string{OrgSingle: "single", OrgOneD: "1d", OrgTwoD: "2d", OrgTwoDInv: "2d-inverse"}

// Names follow OLS_LangE.dll. Values 4 and 5 are unconfirmed (docs/kp-format.md,
// Enums): 4 free editable and 5 backwards in the script enum order, the reverse
// of the DLL's UI order.
var dsNames = map[DataSource]string{
	DSOrdinal: "ordinal", DSEeprom: "eeprom", DSEepromAdd: "eeprom-add", DSEepromSubtract: "eeprom-subtract",
}

var typeNames = map[Type]string{
	1: "u8", 2: "16-hilo", 3: "16-lohi", 4: "32-hilo", 5: "32-lohi", 6: "float-hilo", 7: "float-lohi",
}

// Width returns the element size in bytes, or 0 if unknown.
func (t Type) Width() int {
	switch {
	case t == 1:
		return 1
	case t == 2 || t == 3:
		return 2
	case t >= 4 && t <= 7:
		return 4
	}
	return 0
}

// LE reports little-endian storage.
func (t Type) LE() bool { return t > 1 && t&1 == 1 }

// Float reports IEEE single precision.
func (t Type) Float() bool { return t == 6 || t == 7 }

// FromImage reports whether axis values are read from the image.
func (d DataSource) FromImage() bool { return d >= DSEeprom && d <= DSEepromSubtract }

func name[K ~int32](names map[K]string, k K) string {
	if s, ok := names[k]; ok {
		return s
	}
	return fmt.Sprintf("#%d", int32(k))
}

func (o Org) String() string        { return name(orgNames, o) }
func (t Type) String() string       { return name(typeNames, t) }
func (d DataSource) String() string { return name(dsNames, d) }

func unname[K ~int32](names map[K]string, b []byte, k *K) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	for v, n := range names {
		if n == s {
			*k = v
			return nil
		}
	}
	var v int32
	if _, err := fmt.Sscanf(s, "#%d", &v); err != nil {
		return fmt.Errorf("unknown name %q", s)
	}
	*k = K(v)
	return nil
}

func (o Org) MarshalJSON() ([]byte, error)        { return json.Marshal(o.String()) }
func (t Type) MarshalJSON() ([]byte, error)       { return json.Marshal(t.String()) }
func (d DataSource) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (o *Org) UnmarshalJSON(b []byte) error        { return unname(orgNames, b, o) }
func (t *Type) UnmarshalJSON(b []byte) error       { return unname(typeNames, b, t) }
func (d *DataSource) UnmarshalJSON(b []byte) error { return unname(dsNames, b, d) }
