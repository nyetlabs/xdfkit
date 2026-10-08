// Package kp reads and writes WinOLS map packs ("OLS Map Pack", *.kp) in the two
// layouts ecuxplot's mapdump understands: v1 (header 0x71/0x74, maps inline) and
// v2 (header 0x124/0x149, maps in a deflated zip entry named "intern").
//
// Undecoded fields are named after the preceding named field plus Unk plus
// their hex byte offset from its end in the v2 layout (YUnk29), or Unk plus the
// offset from the start of the file header or a record when nothing named
// precedes them (Unk18); raw regions are named for the region (UnkHeader,
// UnkTrailing). docs/kp-format.md has the details. Each should get a real name
// once its meaning is known. Undecoded runs are kept as Hex blocks so Encode
// reproduces the input.
package kp

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
)

// Layout is the KP container generation.
type Layout int

const (
	V1 Layout = 1
	V2 Layout = 2
)

// File is a parsed map pack. Fields are in file order (see codec for the tags).
type File struct {
	Signature      string  `json:"signature"`
	HeaderID       int32   `json:"headerId"`        // 0x71/0x74 (v1) or 0x124/0x149 (v2)
	HeaderLen      int32   `json:"headerLen"`       // v2: total file length (recomputed by Encode); v1: 0
	Unk18          Hex     `json:"unk18" kp:"hook"` // from 0x18 to the end of the fixed header, 0x5c (v1) or 0x60 (v2)
	Filename       string  `json:"filename"`
	Version        string  `json:"version"`
	UnkHeader      Hex     `json:"unkHeader" kp:"hook"` // between Version and EndOffset
	EndOffset      int32   `json:"endOffset"`           // offset of the end marker, file length - 4 (recomputed by Encode)
	EndOffsetUnk00 Hex     `json:"endOffsetUnk00" kp:"len=12"`
	Project        Project `json:"project"`
}

type Project struct {
	Name         string    `json:"name"`
	NameUnk00    [4]int32  `json:"nameUnk00"`
	NameUnk10    Hex       `json:"nameUnk10,omitzero" kp:"v2,len=4"`
	Version      string    `json:"version"`
	VersionUnk00 [4]int32  `json:"versionUnk00"`
	VersionUnk10 Hex       `json:"versionUnk10,omitzero" kp:"v2,len=4"`
	VersionUnk14 int32     `json:"versionUnk14"`
	VersionUnk18 Hex       `json:"versionUnk18,omitzero" kp:"v2,len=1"`
	UnkIntern    byte      `json:"unkIntern,omitzero" kp:"-"` // v2: first byte of the map block
	Maps         []*Map    `json:"maps" kp:"hook"`
	MapsUnk00    [3]int32  `json:"mapsUnk00"`
	Folders      []*Folder `json:"folders" kp:"count"`
	UnkTrailing  Hex       `json:"unkTrailing" kp:"rest"`

	intern, zip []byte // v2 as read: inflated map block and its zip archive
	zipOff      int    // v2 as read: offset of the zip length
}

type Folder struct {
	ID        int32  `json:"id"`
	IDUnk00   int32  `json:"idUnk00"`
	Name      string `json:"name"`
	NameUnk00 Hex    `json:"nameUnk00" kp:"len=2"`
	NameUnk02 int32  `json:"nameUnk02"`
	NameUnk06 Hex    `json:"nameUnk06,omitzero" kp:"v2,len=15"`
}

const headerLenOff = 0x14

// Parse decodes a map pack.
func Parse(data []byte) (f *File, err error) {
	defer catch(&err)
	f = &File{}
	(&codec{b: data}).record(f)
	return f, nil
}

// Encode serializes f. For v2 the map block is re-deflated only if it changed;
// WinOLS used zlib, so a fresh archive is equivalent but not byte-identical.
// The length-derived fields (HeaderLen in v2, EndOffset) are recomputed and
// stored back into f, so f matches what parsing the output gives.
func (f *File) Encode() (out []byte, err error) {
	defer catch(&err)
	c := &codec{enc: true, want: map[string]bool{"endOffset": true}, found: map[string]Location{}}
	c.record(f)
	if f.Layout() == V2 {
		f.HeaderLen = int32(len(c.b))
		binary.LittleEndian.PutUint32(c.b[headerLenOff:], uint32(f.HeaderLen))
	}
	f.EndOffset = int32(len(c.b) - 4)
	binary.LittleEndian.PutUint32(c.b[c.found["endOffset"].Off:], uint32(f.EndOffset))
	return c.b, nil
}

// Offsets returns where the fields at paths are stored, in one walk. Paths use
// JSON names, as in "project.maps[3].x.dataSource".
func (f *File) Offsets(paths ...string) (locs map[string]Location, err error) {
	defer catch(&err)
	c := &codec{enc: true, want: map[string]bool{}, found: map[string]Location{}}
	for _, p := range paths {
		c.want[p] = true
	}
	c.record(f)
	for _, p := range paths {
		if _, ok := c.found[p]; !ok {
			return nil, fmt.Errorf("no field %q", p)
		}
	}
	return c.found, nil
}

// Offset returns where the field at path is stored (see Offsets).
func (f *File) Offset(path string) (Location, error) {
	locs, err := f.Offsets(path)
	return locs[path], err
}

// Layout reports the container generation, or 0 for an unknown header.
func (f *File) Layout() Layout {
	switch f.HeaderID {
	case 0x71, 0x74:
		return V1
	case 0x124, 0x149:
		return V2
	}
	return 0
}

func (f *File) hook(c *codec, field string) {
	switch field {
	case "Unk18":
		c.layout = f.Layout()
		end := map[Layout]int{V1: 0x5c, V2: 0x60}[c.layout]
		if end == 0 {
			c.fail("unknown kp header 0x%x", f.HeaderID)
		}
		c.raw(&f.Unk18, end-c.pos)
	case "UnkHeader":
		c.opaque(&f.UnkHeader, func() { walkHeader(c, c.layout) })
	}
}

// walkHeader steps over the header after the filename and version strings,
// checking its terminators. The layout follows mapdump's Parser.parseHeader.
func walkHeader(c *codec, l Layout) {
	var s string
	skip := func(n int) { c.take(n) }
	skip(6*4 + 2 + 4)
	c.term("1")
	skip(7*4 + 3*4 + 23*4 + 4 + 4*4 + 2*4 + 2 + 1)
	c.term("2")
	if l == V1 {
		skip(7*4 + 1)
	} else {
		skip(1 + 6*4)
		c.str(&s)
		c.str(&s)
		skip(10 * 4)
		c.term("2a")
		skip(1 + 12*4)
		c.str(&s)
		skip(10*4 + 1)
	}
	skip(0x14*2 + 0x10*2 + 18*4)
	c.term("3")
	skip(4)
	if l == V2 {
		skip(8)
	}
}

// hook codes the maps: inline in v1; in v2 an int length, then a zip holding
// "intern" (UnkIntern, then the maps).
func (p *Project) hook(c *codec, _ string) {
	maps := func(c *codec) {
		c.list(&p.Maps)
		for i, m := range p.Maps {
			m.Index = i
		}
	}
	if c.layout == V1 {
		maps(c)
		return
	}
	if !c.enc {
		p.zipOff = c.pos
		n := c.count(0)
		p.zip = append([]byte(nil), c.take(n)...)
		intern, err := unzipIntern(p.zip)
		if err != nil {
			c.fail("%v", err)
		}
		p.intern = intern
		ic := c.sub(intern)
		ic.u8(&p.UnkIntern)
		maps(ic)
		if ic.pos != len(intern) {
			c.fail("v2 map block: %d bytes after the maps", len(intern)-ic.pos)
		}
		return
	}
	ic := c.sub(nil)
	ic.u8(&p.UnkIntern)
	maps(ic)
	z := p.zip
	if z == nil || !bytes.Equal(ic.b, p.intern) {
		var err error
		if z, err = zipIntern(ic.b); err != nil {
			c.fail("%v", err)
		}
	}
	c.count(len(z))
	c.put(z...)
}

func unzipIntern(b []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("v2 map block: %w", err)
	}
	if len(zr.File) != 1 || zr.File[0].Name != "intern" {
		return nil, fmt.Errorf("v2 map block: expected one entry \"intern\", got %d", len(zr.File))
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// zipIntern builds the archive with the entry metadata every WinOLS 2.24 v2 pack
// carries: deflate, "maximum" flag, version 2.0 (FAT), DOS date 0, archive attribute.
func zipIntern(intern []byte) ([]byte, error) {
	var def bytes.Buffer
	fw, err := flate.NewWriter(&def, flate.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(intern); err != nil {
		return nil, err
	}
	if err := fw.Close(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	w, err := zw.CreateRaw(&zip.FileHeader{
		Name:               "intern",
		Method:             zip.Deflate,
		Flags:              0x2,
		CreatorVersion:     20,
		ReaderVersion:      20,
		ExternalAttrs:      0x20,
		CRC32:              crc32.ChecksumIEEE(intern),
		CompressedSize64:   uint64(def.Len()),
		UncompressedSize64: uint64(len(intern)),
	})
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(def.Bytes()); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
