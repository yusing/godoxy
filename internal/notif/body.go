package notif

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
)

type (
	LogField struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	LogFormat struct {
		string
	}
	LogBody interface {
		Format(format *LogFormat) ([]byte, error)
	}
)

type (
	FieldsBody  []LogField
	ListBody    []string
	MessageBody string
)

var (
	LogFormatMarkdown = &LogFormat{"markdown"}
	LogFormatPlain    = &LogFormat{"plain"}
	LogFormatRawJSON  = &LogFormat{"json"} // internal use only
)

func MakeLogFields(fields ...LogField) LogBody {
	return FieldsBody(fields)
}

func (f *LogFormat) Parse(format string) error {
	switch format {
	case "":
		f.string = LogFormatMarkdown.string
	case LogFormatPlain.string, LogFormatMarkdown.string:
		f.string = format
	default:
		return gperr.Multiline().
			Addf("invalid log format %s, supported formats:", format).
			AddLines(
				LogFormatPlain,
				LogFormatMarkdown,
			)
	}
	return nil
}

func (f FieldsBody) Format(format *LogFormat) ([]byte, error) {
	switch format {
	case LogFormatMarkdown:
		var msg bytes.Buffer
		for _, field := range f {
			msg.WriteString("#### ")
			msg.WriteString(field.Name)
			msg.WriteRune('\n')
			msg.WriteString(field.Value)
			msg.WriteRune('\n')
		}
		return msg.Bytes(), nil
	case LogFormatPlain:
		var msg bytes.Buffer
		for _, field := range f {
			msg.WriteString(field.Name)
			msg.WriteString(": ")
			msg.WriteString(field.Value)
			msg.WriteRune('\n')
		}
		return msg.Bytes(), nil
	case LogFormatRawJSON:
		return json.Marshal(f)
	}
	return nil, fmt.Errorf("unknown format: %v", format)
}

func (l ListBody) Format(format *LogFormat) ([]byte, error) {
	switch format {
	case LogFormatPlain:
		return []byte(strings.Join(l, "\n")), nil
	case LogFormatMarkdown:
		var msg bytes.Buffer
		for _, item := range l {
			msg.WriteString("* ")
			msg.WriteString(item)
			msg.WriteRune('\n')
		}
		return msg.Bytes(), nil
	case LogFormatRawJSON:
		return json.Marshal(l)
	}
	return nil, fmt.Errorf("unknown format: %v", format)
}

func (m MessageBody) Format(format *LogFormat) ([]byte, error) {
	switch format {
	case LogFormatPlain, LogFormatMarkdown:
		return []byte(m), nil
	case LogFormatRawJSON:
		return json.Marshal(m)
	}
	return nil, fmt.Errorf("unknown format: %v", format)
}
