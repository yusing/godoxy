package serialization

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"

	strutils "github.com/yusing/goutils/strings"
)

// json/v2 has no default encoding for time.Duration. Keep v1's nanosecond
// numbers so existing API and stored JSON (health, HTTP timeouts, ACL) still round-trip.
var jsonOpts = jsonv2.JoinOptions(jsonv1.FormatDurationAsNano(true))

func setupJSONV2() {
	strutils.SetJSONMarshaler(func(v any) ([]byte, error) {
		return jsonv2.Marshal(v, jsonOpts)
	})
	strutils.SetJSONUnmarshaler(func(data []byte, v any) error {
		return jsonv2.Unmarshal(data, v, jsonOpts)
	})
	strutils.SetJSONMarshalIndent(func(v any, prefix, indent string) ([]byte, error) {
		return jsonv2.Marshal(v, jsonOpts, jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent))
	})
	strutils.SetJSONNewEncoder(func(w io.Writer) strutils.Encoder {
		return &jsonEncoder{w: w}
	})
	strutils.SetJSONNewDecoder(func(r io.Reader) strutils.Decoder {
		return &jsonDecoder{dec: jsontext.NewDecoder(r)}
	})
	strutils.SetJSONValid(func(b []byte) bool {
		return jsontext.Value(b).IsValid()
	})
	strutils.SetJSONMarshalString(func(v any) (string, error) {
		b, err := jsonv2.Marshal(v, jsonOpts)
		return string(b), err
	})
	strutils.SetJSONUnmarshalString(func(data string, v any) error {
		return jsonv2.Unmarshal([]byte(data), v, jsonOpts)
	})
	strutils.SetJSONValidString(func(s string) bool {
		return jsontext.Value([]byte(s)).IsValid()
	})
}

type jsonEncoder struct {
	w          io.Writer
	prefix     string
	indent     string
	escapeHTML bool
}

func (e *jsonEncoder) Encode(v any) error {
	opts := []jsonv2.Options{jsonOpts}
	if e.prefix != "" || e.indent != "" {
		opts = append(opts, jsontext.WithIndentPrefix(e.prefix), jsontext.WithIndent(e.indent))
	}
	if e.escapeHTML {
		opts = append(opts, jsontext.EscapeForHTML(true))
	}
	return jsonv2.MarshalEncode(jsontext.NewEncoder(e.w), v, opts...)
}

func (e *jsonEncoder) SetEscapeHTML(escape bool) {
	e.escapeHTML = escape
}

func (e *jsonEncoder) SetIndent(prefix, indent string) {
	e.prefix = prefix
	e.indent = indent
}

type jsonDecoder struct {
	dec *jsontext.Decoder
}

func (d *jsonDecoder) Decode(v any) error {
	return jsonv2.UnmarshalDecode(d.dec, v, jsonOpts)
}
