package model

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Extremes are scanned cell lows/highs from a flash image (-i). KP/XDF ignore it.
type Extremes struct {
	Min    float64 `json:"min" doc:"Lowest cell in display units."`
	Max    float64 `json:"max" doc:"Highest cell in display units."`
	RawMin string  `json:"rawMin" doc:"Raw cell behind min (0x hex)."`
	RawMax string  `json:"rawMax" doc:"Raw cell behind max (0x hex)."`
}

// ScanExtremes reads o's cells in image. Nil when there are no cells. RawMin/Max
// are the cells behind the converted min/max (a negative factor swaps ends).
func ScanExtremes(o *Object, image []byte) (*Extremes, error) {
	w := o.Data.Bits / 8
	n := o.Rows * o.Cols
	if n == 0 || w == 0 {
		return nil, nil
	}
	start := int(o.Address)
	if start+n*w > len(image) {
		return nil, fmt.Errorf("cells at 0x%x run past the image end", uint32(o.Address))
	}
	var order binary.ByteOrder = binary.LittleEndian
	if o.Data.Endian == "big" {
		order = binary.BigEndian
	}
	var lo, hi float64
	var rlo, rhi uint64
	for i := range n {
		c := image[start+i*w:]
		var u uint64
		switch w {
		case 1:
			u = uint64(c[0])
		case 2:
			u = uint64(order.Uint16(c))
		default:
			u = uint64(order.Uint32(c))
		}
		v := float64(u)
		switch {
		case o.Data.Float:
			v = float64(math.Float32frombits(uint32(u)))
		case o.Data.Signed:
			v = float64(int64(u<<(64-8*w)) >> (64 - 8*w))
		}
		if i == 0 || v < lo {
			lo, rlo = v, u
		}
		if i == 0 || v > hi {
			hi, rhi = v, u
		}
	}
	a, b := o.Value.Conversion.Apply(lo), o.Value.Conversion.Apply(hi)
	if a > b {
		a, b = b, a
		rlo, rhi = rhi, rlo
	}
	return &Extremes{Min: a, Max: b, RawMin: fmt.Sprintf("0x%x", rlo), RawMax: fmt.Sprintf("0x%x", rhi)}, nil
}

// ApplyScan sets Extremes on each object from image (clears when no cells).
func ApplyScan(m *Model, image []byte) error {
	for _, o := range m.Objects {
		e, err := ScanExtremes(o, image)
		if err != nil {
			return fmt.Errorf("%s: %w", o.Key, err)
		}
		o.Extremes = e
	}
	return nil
}
