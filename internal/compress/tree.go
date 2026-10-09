package compress

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// node is an order-preserving JSON tree: keys keep their order and non-string scalars keep
// their literal text, so a response can be rewritten without changing its shape.
type node struct {
	kind byte // 's' string, 'o' object, 'a' array, 'r' raw scalar (number, bool, null)
	s    string
	raw  string
	keys []string
	kids []*node
}

func parseTree(data []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := parseNode(dec, 0)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return n, nil
}

func parseNode(dec *json.Decoder, depth int) (*node, error) {
	if depth > 200 {
		return nil, errors.New("too deep")
	}
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &node{kind: 'a'}
		if t == '{' {
			n.kind = 'o'
		}
		for dec.More() {
			if n.kind == 'o' {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := kt.(string)
				if !ok {
					return nil, errors.New("bad key")
				}
				n.keys = append(n.keys, k)
			}
			kid, err := parseNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			n.kids = append(n.kids, kid)
		}
		if _, err := dec.Token(); err != nil { // closing delimiter
			return nil, err
		}
		return n, nil
	case string:
		return &node{kind: 's', s: t}, nil
	case json.Number:
		return &node{kind: 'r', raw: t.String()}, nil
	case bool:
		if t {
			return &node{kind: 'r', raw: "true"}, nil
		}
		return &node{kind: 'r', raw: "false"}, nil
	case nil:
		return &node{kind: 'r', raw: "null"}, nil
	}
	return nil, errors.New("unexpected token")
}

func writeString(b *bytes.Buffer, s string) {
	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	b.Truncate(b.Len() - 1) // Encode appends '\n'
}

func (n *node) write(b *bytes.Buffer) {
	switch n.kind {
	case 's':
		writeString(b, n.s)
	case 'r':
		b.WriteString(n.raw)
	case 'o':
		b.WriteByte('{')
		for i, k := range n.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			writeString(b, k)
			b.WriteByte(':')
			n.kids[i].write(b)
		}
		b.WriteByte('}')
	case 'a':
		b.WriteByte('[')
		for i, kid := range n.kids {
			if i > 0 {
				b.WriteByte(',')
			}
			kid.write(b)
		}
		b.WriteByte(']')
	}
}

func (n *node) get(key string) *node {
	for i, k := range n.keys {
		if k == key {
			return n.kids[i]
		}
	}
	return nil
}

func (n *node) isTrue() bool { return n != nil && n.kind == 'r' && n.raw == "true" }

// scan reports whether the response carries an image or looks like a failure.
func scan(n *node) (image, failed bool) {
	switch n.kind {
	case 'o':
		for i, k := range n.keys {
			kid := n.kids[i]
			switch k {
			case "isImage":
				image = image || kid.isTrue()
			case "is_error", "isError", "interrupted":
				failed = failed || kid.isTrue()
			case "success":
				failed = failed || (kid.kind == 'r' && kid.raw == "false")
			case "exitCode", "exit_code", "returncode", "returnCode":
				if kid.kind == 'r' && kid.raw != "0" && kid.raw != "null" && kid.raw != "false" && kid.raw != "true" {
					failed = true
				}
			}
			im, f := scan(kid)
			image, failed = image || im, failed || f
		}
	case 'a':
		for _, kid := range n.kids {
			im, f := scan(kid)
			image, failed = image || im, failed || f
		}
	}
	return
}

func isImageBlock(n *node) bool {
	if n.kind != 'o' {
		return false
	}
	t := n.get("type")
	return t != nil && t.kind == 's' && (t.s == "image" || t.s == "audio" || t.s == "resource_blob")
}
