package json

import (
	"encoding"
	"fmt"
	"reflect"
	"time"

	"github.com/yusing/go-proxy/internal/utils/strutils"
)

func isIntFloat(t reflect.Kind) bool {
	return t >= reflect.Bool && t <= reflect.Float64
}

func appendStringRepr(v reflect.Value, buf []byte) []byte { // for json tag `string`
	kind := v.Kind()
	if isIntFloat(kind) {
		marshalFunc, _ := marshalFuncByKind[kind]
		buf = append(buf, '"')
		buf = marshalFunc(v, buf)
		buf = append(buf, '"')
		return buf
	}
	switch vv := v.Interface().(type) {
	case fmt.Stringer:
		buf = AppendString(buf, vv.String())
	case encoding.TextMarshaler:
		buf = append(buf, must(vv.MarshalText())...)
	case encoding.TextAppender:
		buf = must(vv.AppendText(buf))
	default:
		panic(fmt.Errorf("tag %q used but type is non-stringable: %s", tagString, v.Type()))
	}
	return buf
}

func appendTime(v reflect.Value, buf []byte) []byte {
	buf = append(buf, '"')
	buf = strutils.AppendTime(v.Interface().(time.Time), buf)
	return append(buf, '"')
}

func appendDuration(v reflect.Value, buf []byte) []byte {
	buf = append(buf, '"')
	buf = strutils.AppendDuration(v.Interface().(time.Duration), buf)
	return append(buf, '"')
}

func appendByteSize(v reflect.Value, buf []byte) []byte {
	buf = append(buf, '"')
	buf = strutils.AppendByteSize(v.Interface().(uint64), buf)
	return append(buf, '"')
}

func appendUnixTime(v reflect.Value, buf []byte) []byte {
	buf = append(buf, '"')
	buf = strutils.AppendTime(time.Unix(v.Interface().(int64), 0), buf)
	return append(buf, '"')
}
