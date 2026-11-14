package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"
)

// pyJSONDumps re-serialises JSON text the way Python's json.dumps does with
// its defaults (separators ", " and ": ", ensure_ascii), keeping object key
// order as written. The upstream benchmark builds its prompt from
// json.dumps(json.load(f)), so this reproduces that text. Numbers are kept as
// written in the file; Python would re-format a float such as 1.50 as 1.5.
func pyJSONDumps(data []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var b strings.Builder
	if err := dumpValue(dec, &b); err != nil {
		return "", err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "", errors.New("pyJSONDumps: trailing data after JSON value")
	}
	return b.String(), nil
}

func dumpValue(dec *json.Decoder, b *strings.Builder) error {
	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("pyJSONDumps: %w", err)
	}
	return dumpToken(dec, b, tok)
}

func dumpToken(dec *json.Decoder, b *strings.Builder, tok json.Token) error {
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			b.WriteByte('{')
			first := true
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return fmt.Errorf("pyJSONDumps: %w", err)
				}
				key, ok := kt.(string)
				if !ok {
					return fmt.Errorf("pyJSONDumps: non-string key %v", kt)
				}
				if !first {
					b.WriteString(", ")
				}
				first = false
				dumpString(b, key)
				b.WriteString(": ")
				if err := dumpValue(dec, b); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("pyJSONDumps: %w", err)
			}
			b.WriteByte('}')
		case '[':
			b.WriteByte('[')
			first := true
			for dec.More() {
				if !first {
					b.WriteString(", ")
				}
				first = false
				if err := dumpValue(dec, b); err != nil {
					return err
				}
			}
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("pyJSONDumps: %w", err)
			}
			b.WriteByte(']')
		default:
			return fmt.Errorf("pyJSONDumps: unexpected delimiter %v", t)
		}
	case string:
		dumpString(b, t)
	case json.Number:
		b.WriteString(t.String())
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case nil:
		b.WriteString("null")
	default:
		return fmt.Errorf("pyJSONDumps: unexpected token %T", tok)
	}
	return nil
}

// dumpString writes s as Python's json.dumps does with ensure_ascii=True.
func dumpString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (r >= 0x7f && r < 0x10000):
			fmt.Fprintf(b, `\u%04x`, r)
		case r >= 0x10000:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(b, `\u%04x\u%04x`, hi, lo)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
