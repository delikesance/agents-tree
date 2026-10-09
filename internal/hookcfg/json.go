package hookcfg

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// obj is an order-preserving JSON object. Values are *obj, []any, string, json.Number, bool or nil.
type obj struct {
	keys []string
	m    map[string]any
}

func newObj() *obj { return &obj{m: map[string]any{}} }

func (o *obj) get(k string) (any, bool) { v, ok := o.m[k]; return v, ok }

func (o *obj) set(k string, v any) {
	if _, ok := o.m[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.m[k] = v
}

func (o *obj) del(k string) {
	if _, ok := o.m[k]; !ok {
		return
	}
	delete(o.m, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			return
		}
	}
}

func parseValue(dec *json.Decoder, depth int) (any, error) {
	if depth > 100 {
		return nil, errors.New("nesting too deep")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			o := newObj()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				v, err := parseValue(dec, depth+1)
				if err != nil {
					return nil, err
				}
				o.set(kt.(string), v) // a duplicate key keeps its first position, last value
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return o, nil
		}
		arr := []any{}
		for dec.More() {
			v, err := parseValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	default:
		return tok, nil // string, json.Number, bool, nil
	}
}

// parseRoot parses a settings document. Empty input yields an empty object.
func parseRoot(src []byte) (*obj, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return newObj(), nil
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	v, err := parseValue(dec, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("invalid JSON: trailing data after the top-level value")
	}
	o, ok := v.(*obj)
	if !ok {
		return nil, errors.New("settings file is not a JSON object")
	}
	return o, nil
}

func quote(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

func marshal(v any, indent string, level int, b *strings.Builder) {
	pad := func(n int) { b.WriteString("\n"); b.WriteString(strings.Repeat(indent, n)) }
	switch t := v.(type) {
	case *obj:
		if len(t.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			pad(level + 1)
			b.WriteString(quote(k))
			b.WriteString(": ")
			marshal(t.m[k], indent, level+1, b)
		}
		pad(level)
		b.WriteByte('}')
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			pad(level + 1)
			marshal(e, indent, level+1, b)
		}
		pad(level)
		b.WriteByte(']')
	case string:
		b.WriteString(quote(t))
	case json.Number:
		b.WriteString(t.String())
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case int:
		fmt.Fprintf(b, "%d", t)
	case nil:
		b.WriteString("null")
	}
}

// render pretty-prints root with the given indent and a trailing newline.
func render(root *obj, indent string) string {
	var b strings.Builder
	marshal(root, indent, 0, &b)
	b.WriteByte('\n')
	return b.String()
}

// detectIndent returns the indentation unit of src (two spaces if it cannot tell).
func detectIndent(src []byte) string {
	for _, line := range strings.Split(string(src), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || len(trimmed) == len(line) {
			continue
		}
		lead := line[:len(line)-len(trimmed)]
		if lead[0] == '\t' {
			return "\t"
		}
		return lead
	}
	return "  "
}
