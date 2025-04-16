package json

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type field struct {
	quotedNameWithCol string

	index      int
	inner      []*field
	hasInner   bool
	isPtr      bool
	omitEmpty  bool // true when json tag is "omitempty" or field is pointer to anonymous struct
	checkEmpty checkEmptyFunc
	marshal    marshalFunc
}

const (
	tagOmitEmpty    = "omitempty"
	tagString       = "string" // https://pkg.go.dev/github.com/yusing/go-proxy/pkg/json#Marshal
	tagByteSize     = "byte_size"
	tagUnixTime     = "unix_time"
	tagUseMarshaler = "use_marshaler"
)

func flattenFields(t reflect.Type) []*field {
	fields, ok := flattenFieldsCache.Load(t)
	if ok {
		return fields
	}

	fields = make([]*field, 0, t.NumField())
	for i := range t.NumField() {
		structField := t.Field(i)
		if !structField.IsExported() {
			continue
		}
		kind := structField.Type.Kind()
		f := &field{
			index: i,
			isPtr: kind == reflect.Pointer,
		}
		jsonTag, ok := structField.Tag.Lookup("json")
		if ok {
			if jsonTag == "-" {
				continue
			}
			parts := strutils.SplitComma(jsonTag)
			if len(parts) > 1 {
				switch parts[1] {
				case tagOmitEmpty:
					f.omitEmpty = true
				case tagString:
					f.marshal = appendStringRepr
				case tagByteSize:
					f.marshal = appendByteSize
				case tagUnixTime:
					f.marshal = appendUnixTime
				case tagUseMarshaler:
					f.marshal = mustAppendWithCustomMarshaler
				default:
					panic(fmt.Errorf("unknown json tag: %s", parts[1]))
				}
				f.quotedNameWithCol = parts[0]
			} else {
				f.quotedNameWithCol = jsonTag
			}
		}

		if f.quotedNameWithCol == "" { // e.g. json:",omitempty"
			f.quotedNameWithCol = structField.Name
		}
		if f.marshal == nil {
			f.marshal = appendMarshal
		}
		if structField.Anonymous {
			t := structField.Type
			if t.Kind() == reflect.Pointer {
				t = t.Elem()
				f.omitEmpty = true
			}
			if t.Kind() == reflect.Struct {
				f.inner = flattenFields(t)
				f.hasInner = len(f.inner) > 0
			}
		}
		fields = append(fields, f)
		if f.omitEmpty {
			f.checkEmpty = checkEmptyFuncs[kind]
		}
		f.quotedNameWithCol = strconv.Quote(f.quotedNameWithCol) + ":"
	}

	flattenFieldsCache.Store(t, fields)
	return fields
}

func (f *field) appendKV(v reflect.Value, buf []byte) []byte {
	return f.marshal(v, append(buf, f.quotedNameWithCol...))
}
