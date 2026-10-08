package canon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// JSONToYAML writes a JSON document as YAML, node for node: keys sorted,
// numbers spelled as in the JSON (so typed numbers survive), strings quoted
// where YAML would read them as another type.
func JSONToYAML(data []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	e := yaml.NewEncoder(&b)
	e.SetIndent(2)
	if err := e.Encode(yamlNode(v)); err != nil {
		return nil, err
	}
	return b.Bytes(), e.Close()
}

func yamlNode(v any) *yaml.Node {
	scalar := func(tag, s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: s} }
	switch v := v.(type) {
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n.Content = append(n.Content, scalar("!!str", k), yamlNode(v[k]))
		}
		return n
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode}
		for _, e := range v {
			n.Content = append(n.Content, yamlNode(e))
		}
		return n
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			return scalar("!!float", string(v))
		}
		return scalar("!!int", string(v))
	case string:
		return scalar("!!str", v)
	case bool:
		return scalar("!!bool", strconv.FormatBool(v))
	}
	return scalar("!!null", "null")
}

// YAMLToJSON converts a YAML document to JSON, node for node, keeping each
// number's spelling where JSON allows it, so canon.Unmarshal and the stamp
// treat it like the JSON it came from. Anchors, aliases, custom tags and
// multiple documents are rejected.
func YAMLToJSON(data []byte) ([]byte, error) {
	d := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := d.Decode(&doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, errors.New("yaml: empty document")
	}
	if err := d.Decode(new(yaml.Node)); err != io.EOF {
		return nil, errors.New("yaml: more than one document")
	}
	var b bytes.Buffer
	if err := writeJSON(&b, doc.Content[0]); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func writeJSON(b *bytes.Buffer, n *yaml.Node) error {
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return fmt.Errorf("yaml line %d: anchors and aliases are not allowed", n.Line)
	}
	switch n.Kind {
	case yaml.MappingNode:
		b.WriteByte('{')
		for i := 0; i < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if k.Kind != yaml.ScalarNode || k.ShortTag() != "!!str" {
				return fmt.Errorf("yaml line %d: keys must be strings", k.Line)
			}
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k.Value)
			b.WriteByte(':')
			if err := writeJSON(b, v); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case yaml.SequenceNode:
		b.WriteByte('[')
		for i, e := range n.Content {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeJSON(b, e); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		return writeScalar(b, n)
	}
	return nil
}

func writeScalar(b *bytes.Buffer, n *yaml.Node) error {
	switch n.ShortTag() {
	case "!!str", "!!timestamp":
		writeString(b, n.Value)
	case "!!bool", "!!null":
		var v any
		if err := n.Decode(&v); err != nil {
			return err
		}
		out, _ := json.Marshal(v)
		b.Write(out)
	case "!!int", "!!float":
		if json.Valid([]byte(n.Value)) {
			b.WriteString(n.Value)
			return nil
		}
		var f float64
		if err := n.Decode(&f); err != nil {
			return err
		}
		out, err := json.Marshal(f)
		if err != nil {
			return fmt.Errorf("yaml line %d: %q is not a JSON number", n.Line, n.Value)
		}
		b.Write(out)
	default:
		return fmt.Errorf("yaml line %d: tag %s is not allowed", n.Line, n.Tag)
	}
	return nil
}
