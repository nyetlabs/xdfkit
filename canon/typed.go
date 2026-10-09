package canon

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Type-guided JSON coding (docs/stamp-and-metadata.md, Typed numbers): Go
// values to the generic tree and back by reflection, so the JSON spelling of a
// number follows its Go type. Whole floats are written with ".0"; integer
// fields accept any literal whose exact value is a whole number in range.

var (
	marshaler   = reflect.TypeFor[json.Marshaler]()
	unmarshaler = reflect.TypeFor[json.Unmarshaler]()
	numberType  = reflect.TypeFor[json.Number]()
	fieldCache  sync.Map // reflect.Type -> []jfield
)

type jfield struct {
	name                string
	index               []int
	omitEmpty, omitZero bool
}

// fields lists the JSON fields of a struct type as encoding/json sees them,
// including those promoted from untagged embedded structs.
func fields(t reflect.Type) []jfield {
	if fs, ok := fieldCache.Load(t); ok {
		return fs.([]jfield)
	}
	var fs []jfield
	at := map[string]int{}
	for _, sf := range reflect.VisibleFields(t) {
		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		ft := sf.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if sf.Anonymous && name == "" && ft.Kind() == reflect.Struct {
			continue
		}
		if !sf.IsExported() || hidden(t, sf.Index) {
			continue
		}
		if name == "" {
			name = sf.Name
		}
		f := jfield{name, sf.Index, strings.Contains(","+opts+",", ",omitempty,"), strings.Contains(","+opts+",", ",omitzero,")}
		if i, ok := at[name]; !ok {
			at[name] = len(fs)
			fs = append(fs, f)
		} else if len(f.index) < len(fs[i].index) {
			fs[i] = f
		}
	}
	fieldCache.Store(t, fs)
	return fs
}

// hidden reports whether a promoted field comes through an embedded struct
// that is itself tagged "-" or named, so encoding/json wouldn't promote it.
func hidden(t reflect.Type, index []int) bool {
	for _, i := range index[:len(index)-1] {
		sf := t.Field(i)
		if name, _, _ := strings.Cut(sf.Tag.Get("json"), ","); name != "" {
			return true
		}
		t = sf.Type
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
	}
	return false
}

func empty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

func zero(v reflect.Value) bool {
	if z, ok := v.Interface().(interface{ IsZero() bool }); ok {
		return (v.Kind() != reflect.Pointer || !v.IsNil()) && z.IsZero()
	}
	return v.IsZero()
}

// encode converts v to the generic tree: maps, slices, strings, bools, nil and
// json.Number spelled as jq prints it.
func encode(v reflect.Value) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	t := v.Type()
	if t == numberType {
		return respell(json.Number(v.String()))
	}
	if t.Implements(marshaler) && (t.Kind() != reflect.Pointer || !v.IsNil()) {
		b, err := v.Interface().(json.Marshaler).MarshalJSON()
		if err != nil {
			return nil, err
		}
		tr, err := decode(b)
		if err != nil {
			return nil, err
		}
		return normalize(tr)
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil, nil
		}
		return encode(v.Elem())
	case reflect.Struct:
		obj := map[string]any{}
		for _, f := range fields(t) {
			fv, err := v.FieldByIndexErr(f.index)
			if err != nil || f.omitEmpty && empty(fv) || f.omitZero && zero(fv) {
				continue
			}
			if obj[f.name], err = encode(fv); err != nil {
				return nil, fmt.Errorf("%s: %w", f.name, err)
			}
		}
		return obj, nil
	case reflect.Map:
		if v.IsNil() {
			return nil, nil
		}
		obj := make(map[string]any, v.Len())
		for it := v.MapRange(); it.Next(); {
			k, err := mapKey(it.Key())
			if err != nil {
				return nil, err
			}
			if obj[k], err = encode(it.Value()); err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
		}
		return obj, nil
	case reflect.Slice:
		if v.IsNil() {
			return nil, nil
		}
		if t.Elem().Kind() == reflect.Uint8 {
			return base64.StdEncoding.EncodeToString(v.Bytes()), nil
		}
		fallthrough
	case reflect.Array:
		list := make([]any, v.Len())
		for i := range list {
			var err error
			if list[i], err = encode(v.Index(i)); err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return list, nil
	case reflect.String:
		return v.String(), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return json.Number(strconv.FormatInt(v.Int(), 10)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return json.Number(strconv.FormatUint(v.Uint(), 10)), nil
	case reflect.Float32, reflect.Float64:
		return float(v.Float(), t.Bits())
	}
	return nil, fmt.Errorf("canon: can't encode %s", t)
}

func mapKey(k reflect.Value) (string, error) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(k.Uint(), 10), nil
	}
	return "", fmt.Errorf("canon: can't encode map key %s", k.Type())
}

// float spells a float as jq 1.8 reprints it: whole values with ".0", plain
// shortest digits from 1e-6 up, and "E-n" below.
func float(f float64, bits int) (json.Number, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", fmt.Errorf("canon: %v is not a JSON number", f)
	}
	if f == math.Trunc(f) {
		return json.Number(strconv.FormatFloat(f, 'f', -1, bits) + ".0"), nil
	}
	if math.Abs(f) >= 1e-6 {
		return json.Number(strconv.FormatFloat(f, 'f', -1, bits)), nil
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(f, 'e', -1, bits), "e-")
	return json.Number(mant + "E-" + strings.TrimLeft(exp, "0")), nil
}

// Unmarshal decodes JSON into v, guided by v's type. Integer fields accept any
// literal with an exact whole value in range (16, 16.0, 1.6E1). Unknown object
// fields are errors, except the top-level stamp. Errors name the JSON path.
func Unmarshal(data []byte, v any) error {
	t, err := decode(data)
	if err != nil {
		return err
	}
	if obj, ok := t.(map[string]any); ok {
		delete(obj, StampKey)
	}
	return FromTree(t, v)
}

func pathErr(path, format string, a ...any) error {
	if path == "" {
		path = "."
	}
	return fmt.Errorf("%s: %s", path, fmt.Sprintf(format, a...))
}

func assign(t any, v reflect.Value, path string) error {
	if v.Type() == numberType {
		n, ok := t.(json.Number)
		if !ok {
			return pathErr(path, "expected a number, got %s", kind(t))
		}
		v.SetString(string(n))
		return nil
	}
	if v.CanAddr() && v.Addr().Type().Implements(unmarshaler) {
		b, err := json.Marshal(t)
		if err != nil {
			return pathErr(path, "%v", err)
		}
		if err := v.Addr().Interface().(json.Unmarshaler).UnmarshalJSON(b); err != nil {
			return pathErr(path, "%v", err)
		}
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer:
		if t == nil {
			v.SetZero()
			return nil
		}
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		return assign(t, v.Elem(), path)
	case reflect.Interface:
		if v.NumMethod() != 0 {
			return pathErr(path, "can't decode into %s", v.Type())
		}
		if t == nil {
			v.SetZero()
		} else {
			v.Set(reflect.ValueOf(t))
		}
		return nil
	}
	if t == nil {
		v.SetZero()
		return nil
	}
	switch v.Kind() {
	case reflect.Struct:
		obj, ok := t.(map[string]any)
		if !ok {
			return pathErr(path, "expected an object, got %s", kind(t))
		}
		byName := map[string]jfield{}
		for _, f := range fields(v.Type()) {
			byName[f.name] = f
		}
		for k, sub := range obj {
			f, ok := byName[k]
			if !ok {
				return pathErr(join(path, k), "unknown field")
			}
			if err := assign(sub, fieldAlloc(v, f.index), join(path, k)); err != nil {
				return err
			}
		}
	case reflect.Map:
		obj, ok := t.(map[string]any)
		if !ok {
			return pathErr(path, "expected an object, got %s", kind(t))
		}
		if v.Type().Key().Kind() != reflect.String {
			return pathErr(path, "can't decode into %s", v.Type())
		}
		m := reflect.MakeMapWithSize(v.Type(), len(obj))
		for k, sub := range obj {
			e := reflect.New(v.Type().Elem()).Elem()
			if err := assign(sub, e, join(path, k)); err != nil {
				return err
			}
			m.SetMapIndex(reflect.ValueOf(k).Convert(v.Type().Key()), e)
		}
		v.Set(m)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			s, ok := t.(string)
			if !ok {
				return pathErr(path, "expected a base64 string, got %s", kind(t))
			}
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				return pathErr(path, "%v", err)
			}
			v.SetBytes(b)
			return nil
		}
		list, ok := t.([]any)
		if !ok {
			return pathErr(path, "expected an array, got %s", kind(t))
		}
		v.Set(reflect.MakeSlice(v.Type(), len(list), len(list)))
		return assignList(list, v, path)
	case reflect.Array:
		list, ok := t.([]any)
		if !ok {
			return pathErr(path, "expected an array, got %s", kind(t))
		}
		if len(list) != v.Len() {
			return pathErr(path, "expected %d elements, got %d", v.Len(), len(list))
		}
		return assignList(list, v, path)
	case reflect.String:
		s, ok := t.(string)
		if !ok {
			return pathErr(path, "expected a string, got %s", kind(t))
		}
		v.SetString(s)
	case reflect.Bool:
		b, ok := t.(bool)
		if !ok {
			return pathErr(path, "expected true or false, got %s", kind(t))
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return assignInt(t, v, path)
	case reflect.Float32, reflect.Float64:
		n, ok := t.(json.Number)
		if !ok {
			return pathErr(path, "expected a number, got %s", kind(t))
		}
		f, err := strconv.ParseFloat(string(n), v.Type().Bits())
		if err != nil {
			return pathErr(path, "%s is out of range for %s", n, v.Type())
		}
		v.SetFloat(f)
	default:
		return pathErr(path, "can't decode into %s", v.Type())
	}
	return nil
}

func assignList(list []any, v reflect.Value, path string) error {
	for i, sub := range list {
		if err := assign(sub, v.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

func assignInt(t any, v reflect.Value, path string) error {
	n, ok := t.(json.Number)
	if !ok {
		return pathErr(path, "expected an integer, got %s", kind(t))
	}
	r, ok := new(big.Rat).SetString(string(n))
	if !ok || !r.IsInt() {
		return pathErr(path, "%s is not an integer", n)
	}
	i := r.Num()
	bits := v.Type().Bits()
	switch v.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if i.Sign() < 0 || i.BitLen() > bits {
			return pathErr(path, "%s is out of range for %s", n, v.Type())
		}
		v.SetUint(i.Uint64())
	default:
		lo := new(big.Int).Lsh(big.NewInt(-1), uint(bits-1))
		hi := new(big.Int).Sub(new(big.Int).Neg(lo), big.NewInt(1))
		if i.Cmp(lo) < 0 || i.Cmp(hi) > 0 {
			return pathErr(path, "%s is out of range for %s", n, v.Type())
		}
		v.SetInt(i.Int64())
	}
	return nil
}

// fieldAlloc is FieldByIndex, allocating nil embedded struct pointers.
func fieldAlloc(v reflect.Value, index []int) reflect.Value {
	for i, x := range index {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func kind(t any) string {
	switch t := t.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return string(t)
	case string:
		return "a string"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return fmt.Sprintf("%T", t)
}
