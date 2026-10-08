package kp

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// codec walks one buffer (the file, or the inflated "intern" entry for v2 maps)
// in either direction, so every record layout is written once. Decoding fails
// on any value it could not re-encode to the same bytes.
//
// Records are Go structs coded field by field, in declaration order, by
// reflection. Integers are little-endian, bools one byte (0 or 1), strings
// length-prefixed ISO-8859-15 (see str), arrays element by element. Field tag
// `kp:"opt,opt..."` options:
//
//   - not stored (derived or bookkeeping); unexported fields are skipped too
//     v1, v2   stored only in that layout
//     len=N    Hex: N raw bytes
//     count    slice: int32 element count, then the elements
//     bytes    slice: int32 byte length, then the elements
//     rest     Hex: everything up to the end of the buffer
//     hook     coded by the struct's hook method
//
// A struct implementing checker has check called after its fields are coded.
type codec struct {
	b      []byte
	pos    int
	enc    bool
	layout Layout

	// Offset lookup: path is the JSON path of the field being coded, tracked
	// only while want is set; found collects the offsets of wanted paths.
	want  map[string]bool
	path  string
	found map[string]Location
	block bool // coding the v2 map block
}

// Location is where a field is stored: in the file, or for v2 maps in the
// inflated map block.
type Location struct {
	Off        int
	InMapBlock bool
}

// hooker codes the fields of its struct tagged `kp:"hook"`.
type hooker interface{ hook(c *codec, field string) }

// checker validates or derives fields after its struct is coded.
type checker interface{ check(c *codec) }

// sub returns a codec for another buffer that shares c's layout and offset lookup.
func (c *codec) sub(b []byte) *codec {
	return &codec{b: b, enc: c.enc, layout: c.layout, want: c.want, path: c.path, found: c.found, block: true}
}

type field struct {
	index      int
	goName     string
	name       string // JSON name
	only       Layout
	n          int
	skip, hook bool
	count      bool
	bytes      bool
	rest       bool
}

var fieldCache sync.Map // reflect.Type -> []field

func fieldsOf(t reflect.Type) []field {
	if fs, ok := fieldCache.Load(t); ok {
		return fs.([]field)
	}
	var fs []field
	for i := range t.NumField() {
		sf := t.Field(i)
		f := field{index: i, goName: sf.Name, name: strings.Split(sf.Tag.Get("json"), ",")[0]}
		if f.name == "" {
			f.name = sf.Name
		}
		f.skip = !sf.IsExported()
		for _, o := range strings.Split(sf.Tag.Get("kp"), ",") {
			switch {
			case o == "":
			case o == "-":
				f.skip = true
			case o == "v1":
				f.only = V1
			case o == "v2":
				f.only = V2
			case o == "count":
				f.count = true
			case o == "bytes":
				f.bytes = true
			case o == "rest":
				f.rest = true
			case o == "hook":
				f.hook = true
			case strings.HasPrefix(o, "len="):
				n, err := strconv.Atoi(o[4:])
				if err != nil {
					panic(fmt.Sprintf("kp: %s.%s: bad tag %q", t, sf.Name, o))
				}
				f.n = n
			default:
				panic(fmt.Sprintf("kp: %s.%s: unknown tag option %q", t, sf.Name, o))
			}
		}
		fs = append(fs, f)
	}
	fieldCache.Store(t, fs)
	return fs
}

// enter appends name to the offset-lookup path and returns the previous path.
func (c *codec) enter(name string) string {
	old := c.path
	if c.want == nil {
		return old
	}
	switch {
	case old == "":
		c.path = name
	case name[0] == '[':
		c.path = old + name
	default:
		c.path = old + "." + name
	}
	if _, done := c.found[c.path]; c.want[c.path] && !done {
		c.found[c.path] = Location{c.pos, c.block}
	}
	return old
}

// record codes the struct p points to.
func (c *codec) record(p any) { c.structFields(reflect.ValueOf(p).Elem()) }

// list codes the slice p points to, with an element count.
func (c *codec) list(p any) { c.value(reflect.ValueOf(p).Elem(), field{count: true}) }

func (c *codec) structFields(v reflect.Value) {
	for _, f := range fieldsOf(v.Type()) {
		if f.skip || (f.only != 0 && f.only != c.layout) {
			continue
		}
		old := c.enter(f.name)
		if f.hook {
			v.Addr().Interface().(hooker).hook(c, f.goName)
		} else {
			c.value(v.Field(f.index), f)
		}
		c.path = old
	}
	if ck, ok := v.Addr().Interface().(checker); ok {
		ck.check(c)
	}
}

func (c *codec) value(v reflect.Value, f field) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		c.value(v.Elem(), f)
	case reflect.Struct:
		c.structFields(v)
	case reflect.Bool:
		x := v.Bool()
		c.bool(&x)
		v.SetBool(x)
	case reflect.Uint8:
		x := byte(v.Uint())
		c.u8(&x)
		v.SetUint(uint64(x))
	case reflect.Int16:
		x := int16(v.Int())
		c.i16(&x)
		v.SetInt(int64(x))
	case reflect.Int32:
		x := int32(v.Int())
		c.i32(&x)
		v.SetInt(int64(x))
	case reflect.Uint32:
		x := uint32(v.Uint())
		c.u32(&x)
		v.SetUint(uint64(x))
	case reflect.Float64:
		x := v.Float()
		c.f64(&x)
		v.SetFloat(x)
	case reflect.String:
		x := v.String()
		c.str(&x)
		v.SetString(x)
	case reflect.Array:
		c.elems(v)
	case reflect.Slice:
		c.slice(v, f)
	default:
		panic(fmt.Sprintf("kp: can't code %s", v.Type()))
	}
}

func (c *codec) slice(v reflect.Value, f field) {
	switch {
	case f.n > 0 || f.rest:
		h := v.Addr().Interface().(*Hex)
		if f.rest && !c.enc {
			f.n = len(c.b) - c.pos
		} else if f.rest {
			f.n = len(*h)
		}
		c.raw(h, f.n)
		return
	case f.count:
		n := c.count(v.Len())
		if !c.enc {
			v.Set(reflect.MakeSlice(v.Type(), n, n))
		}
	case f.bytes:
		size := int(v.Type().Elem().Size())
		n := c.count(v.Len() * size)
		if n%size != 0 {
			c.pos -= 4
			c.fail("list length %d is not a multiple of %d", n, size)
		}
		if !c.enc {
			v.Set(reflect.MakeSlice(v.Type(), n/size, n/size))
		}
	default:
		panic(fmt.Sprintf("kp: slice %s needs len=, count, bytes or rest", v.Type()))
	}
	c.elems(v)
}

func (c *codec) elems(v reflect.Value) {
	for i := range v.Len() {
		old := c.enter("[" + strconv.Itoa(i) + "]")
		c.value(v.Index(i), field{})
		c.path = old
	}
}

// ParseError reports where decoding or encoding stopped.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string { return fmt.Sprintf("@0x%x: %s", e.Pos, e.Msg) }

func (c *codec) fail(format string, a ...any) {
	panic(&ParseError{c.pos, fmt.Sprintf(format, a...)})
}

// catch turns a codec panic into *err.
func catch(err *error) {
	if p := recover(); p != nil {
		pe, ok := p.(*ParseError)
		if !ok {
			panic(p)
		}
		*err = pe
	}
}

func (c *codec) take(n int) []byte {
	if n < 0 || c.pos+n > len(c.b) {
		c.fail("short read: need %d bytes, %d left", n, len(c.b)-c.pos)
	}
	v := c.b[c.pos : c.pos+n : c.pos+n]
	c.pos += n
	return v
}

func (c *codec) put(v ...byte) {
	c.b = append(c.b, v...)
	c.pos = len(c.b)
}

// Hex is an undecoded byte run, shown as hex in JSON.
type Hex []byte

func (h Hex) MarshalJSON() ([]byte, error) { return json.Marshal(hex.EncodeToString(h)) }

func (h *Hex) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := hex.DecodeString(s)
	*h = v
	return err
}

func (c *codec) raw(p *Hex, n int) {
	if c.enc {
		if len(*p) != n {
			c.fail("raw block: have %d bytes, layout needs %d", len(*p), n)
		}
		c.put(*p...)
	} else {
		*p = append(Hex(nil), c.take(n)...)
	}
}

// opaque keeps the bytes walk consumes as one raw block.
func (c *codec) opaque(p *Hex, walk func()) {
	if c.enc {
		c.put(*p...)
		return
	}
	start := c.pos
	walk()
	*p = append(Hex(nil), c.b[start:c.pos]...)
}

func (c *codec) u8(p *byte) {
	if c.enc {
		c.put(*p)
	} else {
		*p = c.take(1)[0]
	}
}

func (c *codec) bool(p *bool) {
	var v byte
	if *p {
		v = 1
	}
	c.u8(&v)
	if v > 1 {
		c.pos--
		c.fail("bool byte 0x%x", v)
	}
	*p = v == 1
}

func (c *codec) i16(p *int16) {
	if c.enc {
		c.put(binary.LittleEndian.AppendUint16(nil, uint16(*p))...)
	} else {
		*p = int16(binary.LittleEndian.Uint16(c.take(2)))
	}
}

func (c *codec) u32(p *uint32) {
	if c.enc {
		c.put(binary.LittleEndian.AppendUint32(nil, *p)...)
	} else {
		*p = binary.LittleEndian.Uint32(c.take(4))
	}
}

func (c *codec) i32(p *int32) {
	v := uint32(*p)
	c.u32(&v)
	*p = int32(v)
}

func (c *codec) f64(p *float64) {
	if c.enc {
		c.put(binary.LittleEndian.AppendUint64(nil, math.Float64bits(*p))...)
	} else {
		*p = math.Float64frombits(binary.LittleEndian.Uint64(c.take(8)))
	}
}

// count codes an element count: written from n, returned when read.
func (c *codec) count(n int) int {
	v := int32(n)
	c.i32(&v)
	if v < 0 {
		c.pos -= 4
		c.fail("negative count %d", v)
	}
	return int(v)
}

func (c *codec) term(what string) {
	v := int32(-1)
	c.i32(&v)
	if v != -1 {
		c.pos -= 4
		c.fail("missing terminator %s (got 0x%x)", what, uint32(v))
	}
}

// str codes a length-prefixed ISO-8859-15 string; non-empty strings carry a trailing NUL.
func (c *codec) str(p *string) {
	if c.enc {
		b, err := toLatin9(*p)
		if err != nil {
			c.fail("%v", err)
		}
		c.count(len(b))
		if len(b) > 0 {
			c.put(append(b, 0)...)
		}
		return
	}
	n := c.count(0)
	if n == 0 {
		*p = ""
		return
	}
	if n > len(c.b)-c.pos {
		c.pos -= 4
		c.fail("bad string length %d", n)
	}
	*p = latin9(c.take(n))
	if c.take(1)[0] != 0 {
		c.pos--
		c.fail("string not NUL terminated")
	}
}

var latin9Diff = map[byte]rune{
	0xa4: '€', 0xa6: 'Š', 0xa8: 'š', 0xb4: 'Ž', 0xb8: 'ž', 0xbc: 'Œ', 0xbd: 'œ', 0xbe: 'Ÿ',
}

var latin9Rev = func() map[rune]byte {
	m := make(map[rune]byte, len(latin9Diff))
	for b, r := range latin9Diff {
		m[r] = b
	}
	return m
}()

func latin9(b []byte) string {
	rs := make([]rune, len(b))
	for i, c := range b {
		if r, ok := latin9Diff[c]; ok {
			rs[i] = r
		} else {
			rs[i] = rune(c)
		}
	}
	return string(rs)
}

func toLatin9(s string) ([]byte, error) {
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if v, ok := latin9Rev[r]; ok {
			b = append(b, v)
		} else if _, taken := latin9Diff[byte(r)]; r < 0x100 && !taken {
			b = append(b, byte(r))
		} else {
			return nil, fmt.Errorf("%q: %U is not in ISO-8859-15", s, r)
		}
	}
	return b, nil
}
