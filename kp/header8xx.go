package kp

import (
	"encoding/binary"
)

// code8xxFolders reads the 8xx folder table: each entry is id, builtIn, name,
// a 12-byte prefix, a length-prefixed blob, then a short separator before the
// next entry. Encode is not supported.
func (p *Project) code8xxFolders(c *codec) {
	if c.enc {
		c.fail("OLS5 folder encode is not supported yet")
	}
	n := c.count(0)
	p.Folders = make([]*Folder, 0, n)
	for i := 0; i < n; i++ {
		if i > 0 {
			if !skip8xxFolderSep(c) {
				c.fail("8xx folder %d: separator", i)
			}
		}
		fd := &Folder{}
		c.i32(&fd.ID)
		c.i32(&fd.BuiltIn)
		c.str(&fd.Name)
		if c.pos+16 > len(c.b) {
			c.fail("8xx folder %d: blob header", i)
		}
		_ = c.take(12)
		var slen uint32
		c.u32(&slen)
		if int(slen) > len(c.b)-c.pos {
			c.fail("8xx folder %d: blob length %d", i, slen)
		}
		c.raw(&fd.Pad, int(slen))
		p.Folders = append(p.Folders, fd)
	}
}

// skip8xxFolderSep advances past the bytes between two 8xx folders.
// Usual form: 16–17 zero bytes, then -1 or a small int, then two zero int32s.
func skip8xxFolderSep(c *codec) bool {
	start := c.pos
	for c.pos < len(c.b) && c.b[c.pos] == 0 {
		c.pos++
		if c.pos-start > 24 {
			break
		}
	}
	if c.pos+12 > len(c.b) {
		return false
	}
	v := int32(binary.LittleEndian.Uint32(c.b[c.pos:]))
	// -1 is common; Tables is followed by 2.
	if v != -1 && (v < 0 || v > 16) {
		return find8xxFolderStart(c, start)
	}
	c.pos += 4
	for i := 0; i < 2; i++ {
		if c.pos+4 > len(c.b) {
			return false
		}
		if int32(binary.LittleEndian.Uint32(c.b[c.pos:])) != 0 {
			return find8xxFolderStart(c, start)
		}
		c.pos += 4
	}
	return true
}

// find8xxFolderStart scans forward for id/builtIn/nameLen that look like a folder.
func find8xxFolderStart(c *codec, from int) bool {
	for off := from; off+12 < len(c.b); off++ {
		nl := int(int32(binary.LittleEndian.Uint32(c.b[off+8:])))
		if nl < 0 || nl > 300 || off+12+nl >= len(c.b) {
			continue
		}
		if nl > 0 && c.b[off+12+nl] != 0 {
			continue
		}
		name := c.b[off+12 : off+12+nl]
		ok := true
		for _, ch := range name {
			if ch < 0x20 && ch != 0x09 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		c.pos = off
		return true
	}
	return false
}
