package odoo

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Minimal XML-RPC codec (standard library only), sufficient for Odoo's
// /xmlrpc/2/common and /xmlrpc/2/object endpoints.

func encodeValue(b *bytes.Buffer, v any) {
	b.WriteString("<value>")
	switch x := v.(type) {
	case nil:
		b.WriteString("<boolean>0</boolean>")
	case bool:
		if x {
			b.WriteString("<boolean>1</boolean>")
		} else {
			b.WriteString("<boolean>0</boolean>")
		}
	case int:
		fmt.Fprintf(b, "<int>%d</int>", x)
	case int64:
		fmt.Fprintf(b, "<int>%d</int>", x)
	case float64:
		fmt.Fprintf(b, "<double>%s</double>", strconv.FormatFloat(x, 'f', -1, 64))
	case string:
		b.WriteString("<string>")
		_ = xml.EscapeText(b, []byte(x))
		b.WriteString("</string>")
	case time.Time:
		fmt.Fprintf(b, "<dateTime.iso8601>%s</dateTime.iso8601>", x.UTC().Format("20060102T15:04:05"))
	case []byte:
		fmt.Fprintf(b, "<base64>%s</base64>", base64.StdEncoding.EncodeToString(x))
	case []any:
		b.WriteString("<array><data>")
		for _, e := range x {
			encodeValue(b, e)
		}
		b.WriteString("</data></array>")
	case []string:
		b.WriteString("<array><data>")
		for _, e := range x {
			encodeValue(b, e)
		}
		b.WriteString("</data></array>")
	case []int:
		b.WriteString("<array><data>")
		for _, e := range x {
			encodeValue(b, e)
		}
		b.WriteString("</data></array>")
	case map[string]any:
		b.WriteString("<struct>")
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString("<member><name>")
			_ = xml.EscapeText(b, []byte(k))
			b.WriteString("</name>")
			encodeValue(b, x[k])
			b.WriteString("</member>")
		}
		b.WriteString("</struct>")
	default:
		fmt.Fprintf(b, "<string>%v</string>", x)
	}
	b.WriteString("</value>")
}

// EncodeCall renders a methodCall document.
func EncodeCall(method string, params ...any) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	_ = xml.EscapeText(&b, []byte(method))
	b.WriteString("</methodName><params>")
	for _, p := range params {
		b.WriteString("<param>")
		encodeValue(&b, p)
		b.WriteString("</param>")
	}
	b.WriteString("</params></methodCall>")
	return b.Bytes()
}

// Fault is an XML-RPC fault.
type Fault struct {
	Code   int
	String string
}

func (f *Fault) Error() string { return fmt.Sprintf("xmlrpc fault %d: %s", f.Code, f.String) }

// DecodeResponse parses a methodResponse into Go values
// (map[string]any, []any, string, int64, float64, bool, time.Time).
func DecodeResponse(r io.Reader) (any, error) {
	dec := xml.NewDecoder(r)
	var fault bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			switch se.Name.Local {
			case "fault":
				fault = true
			case "value":
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				if fault {
					m, _ := v.(map[string]any)
					code, _ := m["faultCode"].(int64)
					msg, _ := m["faultString"].(string)
					return nil, &Fault{Code: int(code), String: msg}
				}
				return v, nil
			}
		}
	}
}

func decodeValue(dec *xml.Decoder) (any, error) {
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if t.Name.Local == "value" {
				return text.String(), nil
			}
		case xml.StartElement:
			v, err := decodeTyped(dec, t.Name.Local)
			if err != nil {
				return nil, err
			}
			// consume until </value>
			for {
				tok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				if e, ok := tok.(xml.EndElement); ok && e.Name.Local == "value" {
					return v, nil
				}
			}
		}
	}
}

func readText(dec *xml.Decoder, end string) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name.Local == end {
				return b.String(), nil
			}
		}
	}
}

func decodeTyped(dec *xml.Decoder, kind string) (any, error) {
	switch kind {
	case "string":
		return readText(dec, kind)
	case "int", "i4", "i8":
		s, err := readText(dec, kind)
		if err != nil {
			return nil, err
		}
		return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	case "double":
		s, err := readText(dec, kind)
		if err != nil {
			return nil, err
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "boolean":
		s, err := readText(dec, kind)
		return strings.TrimSpace(s) == "1", err
	case "dateTime.iso8601":
		s, err := readText(dec, kind)
		if err != nil {
			return nil, err
		}
		return time.Parse("20060102T15:04:05", strings.TrimSpace(s))
	case "base64":
		s, err := readText(dec, kind)
		if err != nil {
			return nil, err
		}
		return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	case "nil":
		_, err := readText(dec, kind)
		return nil, err
	case "array":
		var out []any
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "value" {
					v, err := decodeValue(dec)
					if err != nil {
						return nil, err
					}
					out = append(out, v)
				}
			case xml.EndElement:
				if t.Name.Local == "array" {
					if out == nil {
						out = []any{}
					}
					return out, nil
				}
			}
		}
	case "struct":
		out := map[string]any{}
		var name string
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "name":
					if name, err = readText(dec, "name"); err != nil {
						return nil, err
					}
				case "value":
					v, err := decodeValue(dec)
					if err != nil {
						return nil, err
					}
					out[name] = v
				}
			case xml.EndElement:
				if t.Name.Local == "struct" {
					return out, nil
				}
			}
		}
	}
	return readText(dec, kind)
}
