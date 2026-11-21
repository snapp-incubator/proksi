package main

import (
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
)

// xmlBytesEqual compares the XML in two byte slices semantically. Comments,
// processing instructions, directives, insignificant whitespace and attribute
// order are ignored; element names (with namespaces), attributes and
// character data must match.
func xmlBytesEqual(a, b []byte) (bool, error) {
	ta, err := xmlTokens(a)
	if err != nil {
		return false, err
	}

	tb, err := xmlTokens(b)
	if err != nil {
		return false, err
	}

	if len(ta) != len(tb) {
		return false, nil
	}
	for i := range ta {
		if ta[i] != tb[i] {
			return false, nil
		}
	}

	return true, nil
}

// xmlTokens returns a canonical string form of every significant token.
func xmlTokens(data []byte) ([]string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	var out []string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			attrs := make([]string, 0, len(t.Attr))
			for _, at := range t.Attr {
				attrs = append(attrs, "{"+at.Name.Space+"}"+at.Name.Local+"="+at.Value)
			}
			sort.Strings(attrs)
			out = append(out, "<{"+t.Name.Space+"}"+t.Name.Local+" "+strings.Join(attrs, "\x00"))
		case xml.EndElement:
			out = append(out, "</{"+t.Name.Space+"}"+t.Name.Local)
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				out = append(out, "#"+s)
			}
		}
	}
}
