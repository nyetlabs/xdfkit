// Package kp reads and writes WinOLS map packs ("OLS Map Pack", *.kp).
// The int32 at offset 0x10 is a decimal header id: 113/116 inline maps;
// 245/292/329/503/597 zip "intern". 245, 503, and 597 are read-only until a
// save oracle exists. Header hybrids: docs/kp-format.md.
//
// Fields whose meaning isn't known get the most likely name, marked
// "(unconfirmed)" in a comment; constants with no known meaning are named Pad
// (zeros), Flag or Tag after the field they follow. docs/kp-format.md has the
// details. Undecoded runs are kept as Hex blocks so Encode reproduces the input.
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

// Layout is a shared record walk, not a WinOLS version (see headerGroup).
type Layout int

const (
	V1   Layout = 1
	V2   Layout = 2
	OLS5 Layout = 5
)

const (
	header113 = 113
	header116 = 116
	header245 = 245
	header292 = 292
	header329 = 329
	header503 = 503
	header597 = 597
)

// File is a parsed map pack. Fields are in file order (see codec for the tags).
type File struct {
	Signature   string  `json:"signature"`
	HeaderID    int32   `json:"headerId"`              // decimal: 113/116, 245, 292/329, or 503/597
	HeaderLen   int32   `json:"headerLen"`             // file length except 113, 116, and 245, where it is 0; recomputed by Encode on 292 and 329
	FixedHeader Hex     `json:"fixedHeader" kp:"hook"` // from 0x18 through 0x5c (113, 116, 245) or 0x60 (the other headers); zeros
	Filename    string  `json:"filename" kp:"hook"`    // 503 and 597: no trailing NUL
	Version     string  `json:"version" kp:"hook"`     // 503 and 597: no trailing NUL
	HeaderBody  Hex     `json:"headerBody" kp:"hook"`  // between Version and EndOffset; partly uninitialized memory
	EndOffset   int32   `json:"endOffset"`             // offset of the end marker (recomputed by Encode except on 503 and 597)
	EndTag      Hex     `json:"endTag" kp:"hook"`      // 12 bytes on 113, 116, 292, 329; 8 bytes on 503 and 597 (unconfirmed)
	Project     Project `json:"project"`
}

type Project struct {
	Name           string    `json:"name"`
	NameTag        [4]int32  `json:"nameTag,omitzero" kp:"v1,v2only"` // constant: a type tag (unconfirmed)
	NamePad        Hex       `json:"namePad,omitzero" kp:"v2only,len=4"`
	NameTrailer    Hex       `json:"nameTrailer,omitzero" kp:"ols5,hook"` // after Name+NUL; 84 bytes on header 503, 93 on 597 (unconfirmed)
	Version        string    `json:"version"`
	VersionTag     [4]int32  `json:"versionTag,omitzero" kp:"v1,v2only"` // constant, equals NameTag: a type tag (unconfirmed)
	VersionPad     Hex       `json:"versionPad,omitzero" kp:"v2only,len=4"`
	VersionTrailer Hex       `json:"versionTrailer,omitzero" kp:"ols5,hook"` // after Version+NUL; 84 bytes on header 503, 93 on 597 (unconfirmed)
	Build          int32     `json:"build"`                                  // constant: a build or format id (unconfirmed)
	BuildFlag      Hex       `json:"buildFlag,omitzero" kp:"v2,len=1"`       // constant 1 (unconfirmed); also OLS5
	InternFlag     byte      `json:"internFlag,omitzero" kp:"-"`             // v2/OLS5: first byte of the map block, 0 (unconfirmed)
	Maps           []*Map    `json:"maps" kp:"hook"`
	MapsTag        [3]int32  `json:"mapsTag"` // constant: a type tag (unconfirmed)
	Folders        []*Folder `json:"folders" kp:"count"`
	Trailing       Hex       `json:"trailing" kp:"rest"` // mostly uninitialized memory

	intern, zip []byte // v2/OLS5 as read: inflated map block and its zip archive
	zipOff      int    // v2/OLS5 as read: offset of the zip length
}

type Folder struct {
	ID      int32  `json:"id"`
	BuiltIn int32  `json:"builtIn"` // 9000 on "My maps", 9001 on "Hexdump", else 0
	Name    string `json:"name"`
	Flags   Hex    `json:"flags" kp:"len=2"`       // 1, 1: expanded and visible (unconfirmed)
	Style   int32  `json:"style"`                  // 0x1000000 (unconfirmed)
	Pad     Hex    `json:"pad,omitzero" kp:"hook"` // v2: 15 bytes; header 245: 14; OLS5: 36 (unconfirmed)
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
// Headers 245, 503, and 597 are read-only until a save oracle exists.
func (f *File) Encode() (out []byte, err error) {
	if f.Layout() == OLS5 || f.HeaderID == header245 {
		return nil, fmt.Errorf("kp: Encode of header %d is not supported yet", f.HeaderID)
	}
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

// headerGroup is the record walk and the kp-defaults.json key for a header id.
func headerGroup(id int32) (Layout, string) {
	switch id {
	case header113, header116:
		return V1, "100"
	case header245, header292, header329:
		return V2, "200+300" // 245 special-cases FixedHeader, walkHeader, intern
	case header503, header597:
		return OLS5, "500"
	}
	return 0, ""
}

// Layout reports the record walk for f.HeaderID, or 0 when the id is unknown.
func (f *File) Layout() Layout {
	l, _ := headerGroup(f.HeaderID)
	return l
}

// DefaultsKey is the kp-defaults.json key for the headers that share f's walk.
func (f *File) DefaultsKey() string {
	_, key := headerGroup(f.HeaderID)
	return key
}

var endTagOLS5 = []byte{0x99, 0x78, 0x00, 0x42, 0x02, 0x00, 0x00, 0x00}

func (f *File) hook(c *codec, field string) {
	switch field {
	case "FixedHeader":
		c.layout = f.Layout()
		c.headerID = f.HeaderID
		end := 0x60
		if c.layout == V1 || c.headerID == header245 {
			end = 0x5c
		} else if c.layout == 0 {
			c.fail("unknown kp header %d", f.HeaderID)
		}
		c.raw(&f.FixedHeader, end-c.pos)
	case "Filename", "Version":
		s := &f.Filename
		if field == "Version" {
			s = &f.Version
		}
		if c.layout == OLS5 {
			c.strRaw(s)
		} else {
			c.str(s)
		}
	case "HeaderBody":
		if c.layout == OLS5 {
			f.codeOLS5HeaderBody(c)
		} else {
			c.opaque(&f.HeaderBody, func() { walkHeader(c, c.layout) })
		}
	case "EndTag":
		n := 12
		if c.layout == OLS5 {
			n = 8
		}
		c.raw(&f.EndTag, n)
	}
}

// codeOLS5HeaderBody keeps the body opaque by locating the WinOLS 5 EndTag magic.
func (f *File) codeOLS5HeaderBody(c *codec) {
	if c.enc {
		c.put(f.HeaderBody...)
		return
	}
	i := bytes.Index(c.b[c.pos:], endTagOLS5)
	if i < 4 {
		c.fail("WinOLS 5 end tag not found")
	}
	c.raw(&f.HeaderBody, i-4) // leave EndOffset (4) before the tag
}

// walkHeader steps over the header after the filename and version strings
// (mapdump Parser.parseHeader; 245 skips term2a/"Binary data", unconfirmed).
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
		if c.headerID == header245 {
			skip(38)
		} else {
			c.term("2a")
			skip(1 + 12*4)
			c.str(&s)
			skip(10*4 + 1)
		}
	}
	skip(0x14*2 + 0x10*2 + 18*4)
	c.term("3")
	skip(4)
	if l == V2 {
		skip(8)
	}
}

// ols5Trailer is the raw block after the project name or version NUL.
// Header 503 is 84 bytes; header 597 is 93.
func ols5Trailer(headerID int32) int {
	if headerID == header503 {
		return 84
	}
	return 93
}

// hook codes the maps: inline in v1; in v2/OLS5 a zip "intern" (OLS5 may leave
// a short trailing block). NameTrailer/VersionTrailer are OLS5-only.
func (p *Project) hook(c *codec, field string) {
	switch field {
	case "NameTrailer":
		c.raw(&p.NameTrailer, ols5Trailer(c.headerID))
		return
	case "VersionTrailer":
		c.raw(&p.VersionTrailer, ols5Trailer(c.headerID))
		return
	}
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
		if c.headerID == header245 {
			ic.layout = V1 // records match 113/116; container stays V2
		}
		ic.u8(&p.InternFlag)
		maps(ic)
		if left := len(intern) - ic.pos; left != 0 {
			if c.layout == OLS5 {
				// OLS5 leaves a short marked-style stub after the maps (65 bytes
				// on header 503, 69 on 597). Consume it so the map block is fully read.
				_ = ic.take(left)
			} else {
				c.fail("v2 map block: %d bytes after the maps", left)
			}
		}
		return
	}
	if c.layout == OLS5 {
		c.fail("OLS5 map block encode is not supported yet")
	}
	ic := c.sub(nil)
	ic.u8(&p.InternFlag)
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

func (fd *Folder) hook(c *codec, field string) {
	if field != "Pad" {
		return
	}
	switch c.layout {
	case V2:
		n := 15
		if c.headerID == header245 {
			n = 14
		}
		c.raw(&fd.Pad, n)
	case OLS5:
		c.raw(&fd.Pad, 36)
	}
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
