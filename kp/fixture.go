package kp

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Shrink returns a minimal pack with at most maxMaps maps and the first two
// folders (when present). V1/V2 use Encode. Headers 245/503/597 rebuild the
// zip from raw map spans recorded while decoding intern (Encode unsupported).
func Shrink(src []byte, maxMaps int) ([]byte, error) {
	if maxMaps < 1 {
		return nil, fmt.Errorf("kp: maxMaps must be >= 1")
	}
	f, err := Parse(src)
	if err != nil {
		return nil, err
	}
	if n := len(f.Project.Maps); n > maxMaps {
		f.Project.Maps = f.Project.Maps[:maxMaps]
	}
	if n := len(f.Project.Folders); n > 2 {
		f.Project.Folders = f.Project.Folders[:2]
	}
	switch f.HeaderID {
	case header113, header116, header292, header329:
		f.Project.zip = nil
		f.Project.intern = nil
		return f.Encode()
	case header245, header503, header597:
		return shrinkZipKP(src, f, maxMaps)
	default:
		return nil, fmt.Errorf("kp: shrink: unsupported header %d", f.HeaderID)
	}
}

func shrinkZipKP(src []byte, f *File, maxMaps int) ([]byte, error) {
	spans, flag, stub, err := mapSpans(f.Project.intern, f.HeaderID)
	if err != nil {
		return nil, err
	}
	if maxMaps > len(spans) {
		maxMaps = len(spans)
	}
	intern := []byte{flag}
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], uint32(maxMaps))
	intern = append(intern, count[:]...)
	for _, sp := range spans[:maxMaps] {
		intern = append(intern, f.Project.intern[sp[0]:sp[1]]...)
	}
	intern = append(intern, stub...)
	z, err := zipIntern(intern)
	if err != nil {
		return nil, err
	}
	return spliceZip(src, f, z)
}

// mapSpans returns each map's [start,end) in intern, the InternFlag, and bytes
// after the last map (OLS5 stub; empty on 245).
func mapSpans(intern []byte, headerID int32) (spans [][2]int, flag byte, stub []byte, err error) {
	defer catch(&err)
	layout := V2
	switch headerID {
	case header245:
		layout = V1
	case header503, header597:
		layout = OLS5
	}
	c := &codec{b: intern, layout: layout, headerID: headerID, block: true}
	c.u8(&flag)
	n := c.count(0)
	spans = make([][2]int, 0, n)
	for i := 0; i < n; i++ {
		start := c.pos
		m := &Map{}
		c.record(m)
		spans = append(spans, [2]int{start, c.pos})
	}
	stub = append([]byte(nil), c.b[c.pos:]...)
	return spans, flag, stub, nil
}

// spliceZip replaces the zip at f.Project.zipOff, appends MapsTag/folders and
// the end marker, and patches EndOffset in the kept prefix.
func spliceZip(src []byte, f *File, z []byte) ([]byte, error) {
	off := f.Project.zipOff
	if off < 4 || off+4 > len(src) {
		return nil, fmt.Errorf("kp: bad zip offset %d", off)
	}
	oldN := int(binary.LittleEndian.Uint32(src[off:]))
	if off+4+oldN > len(src) {
		return nil, fmt.Errorf("kp: zip extends past file")
	}
	tc := &codec{enc: true, layout: f.Layout(), headerID: f.HeaderID}
	for i := range f.Project.MapsTag {
		tc.i32(&f.Project.MapsTag[i])
	}
	tc.list(&f.Project.Folders)
	endMark := []byte{0x33, 0x88, 0x72, 0x98}
	out := append([]byte(nil), src[:off]...)
	var nbuf [4]byte
	binary.LittleEndian.PutUint32(nbuf[:], uint32(len(z)))
	out = append(out, nbuf[:]...)
	out = append(out, z...)
	out = append(out, tc.b...)
	out = append(out, endMark...)

	oldMark := make([]byte, 4)
	binary.LittleEndian.PutUint32(oldMark, uint32(f.EndOffset))
	idx := bytes.Index(out[:off], oldMark)
	if idx < 0 {
		return nil, fmt.Errorf("kp: EndOffset field not found in prefix")
	}
	binary.LittleEndian.PutUint32(out[idx:], uint32(len(out)-4))
	return out, nil
}
