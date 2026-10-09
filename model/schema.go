package model

import (
	"reflect"
	"strconv"
	"strings"

	"go.nyet.org/xdfkit/canon"
)

// Schema returns the JSON Schema (draft 2020-12) of the model, generated from
// the Go types and their tags: json (names, and omitempty/omitzero for
// optional fields), doc (description), enum ("a|b|c") and pattern. The
// committed schema.json must equal it (see the tests).
func Schema() ([]byte, error) {
	return schemaOf(reflect.TypeFor[Model](), "schema.json", "xdfkit map definition model", true)
}

// CategoryTableJSONSchema returns the JSON Schema of the corpus category
// table; the committed categories.schema.json must equal it.
func CategoryTableJSONSchema() ([]byte, error) {
	return schemaOf(reflect.TypeFor[CategoryTable](), "categories.schema.json", "xdfkit category table", false)
}

func schemaOf(t reflect.Type, file, title string, stamp bool) ([]byte, error) {
	g := &schemaGen{defs: map[string]any{}}
	root := g.object(t)
	if stamp {
		root["properties"].(map[string]any)[canon.StampKey] = map[string]any{
			"type":        "object",
			"description": "Edit stamp (docs/stamp-and-metadata.md).",
		}
	}
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = "https://go.nyet.org/xdfkit/model/" + file
	root["title"] = title
	if len(g.defs) > 0 || stamp {
		root["$defs"] = g.defs
	}
	return canon.Marshal(root)
}

type schemaGen struct{ defs map[string]any }

type jsonSchemaer interface{ jsonSchema() map[string]any }

func (g *schemaGen) schema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Pointer {
		return g.schema(t.Elem())
	}
	if s, ok := reflect.Zero(t).Interface().(jsonSchemaer); ok {
		return s.jsonSchema()
	}
	switch t.Kind() {
	case reflect.Struct:
		if _, ok := g.defs[t.Name()]; !ok {
			g.defs[t.Name()] = nil
			g.defs[t.Name()] = g.object(t)
		}
		return map[string]any{"$ref": "#/$defs/" + t.Name()}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": g.schema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": g.schema(t.Elem())}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	}
	panic("model: no schema for " + t.String())
}

func (g *schemaGen) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	required := []any{}
	for i := range t.NumField() {
		sf := t.Field(i)
		name, opts, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if !sf.IsExported() || name == "-" {
			continue
		}
		s := g.schema(sf.Type)
		if d := sf.Tag.Get("doc"); d != "" {
			s["description"] = d
		}
		if p := sf.Tag.Get("pattern"); p != "" {
			s["pattern"] = p
		}
		if e := sf.Tag.Get("enum"); e != "" {
			var vals []any
			for _, v := range strings.Split(e, "|") {
				if n, err := strconv.Atoi(v); err == nil && sf.Type.Kind() != reflect.String {
					vals = append(vals, n)
				} else {
					vals = append(vals, v)
				}
			}
			s["enum"] = vals
		}
		props[name] = s
		if !strings.Contains(","+opts+",", ",omitempty,") && !strings.Contains(","+opts+",", ",omitzero,") {
			required = append(required, name)
		}
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
