package json

import (
	"encoding"
	stdJSON "encoding/json"

	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"time"

	"github.com/puzpuzpuz/xsync/v3"
)

type marshalFunc func(v reflect.Value, buf []byte) []byte

var (
	marshalFuncByKind map[reflect.Kind]marshalFunc

	marshalFuncsByType = newCacheMap[reflect.Type, marshalFunc]()

	nilValue = reflect.ValueOf(nil)
)

func init() {
	marshalFuncByKind = map[reflect.Kind]marshalFunc{
		reflect.String:    appendString,
		reflect.Bool:      appendBool,
		reflect.Int:       appendInt,
		reflect.Int8:      appendInt,
		reflect.Int16:     appendInt,
		reflect.Int32:     appendInt,
		reflect.Int64:     appendInt,
		reflect.Uint:      appendUint,
		reflect.Uint8:     appendUint,
		reflect.Uint16:    appendUint,
		reflect.Uint32:    appendUint,
		reflect.Uint64:    appendUint,
		reflect.Float32:   appendFloat,
		reflect.Float64:   appendFloat,
		reflect.Map:       appendMap,
		reflect.Slice:     appendArray,
		reflect.Array:     appendArray,
		reflect.Struct:    appendStruct,
		reflect.Pointer:   appendPtrInterface,
		reflect.Interface: appendPtrInterface,
	}
	// pre-caching some frequently used types
	marshalFuncsByType.Store(reflect.TypeFor[*url.URL](), appendStringer)
	marshalFuncsByType.Store(reflect.TypeFor[net.IP](), appendStringer)
	marshalFuncsByType.Store(reflect.TypeFor[*net.IPNet](), appendStringer)
	marshalFuncsByType.Store(reflect.TypeFor[time.Time](), appendTime)
	marshalFuncsByType.Store(reflect.TypeFor[time.Duration](), appendDuration)
}

func newCacheMap[K comparable, V any]() *xsync.MapOf[K, V] {
	return xsync.NewMapOf[K, V](
		xsync.WithGrowOnly(),
		xsync.WithPresize(50),
	)
}

func must(buf []byte, err error) []byte {
	if err != nil {
		panic(fmt.Errorf("custom json marshal error: %w", err))
	}
	return buf
}

func appendMarshalAny(v any, buf []byte) []byte {
	return appendMarshal(reflect.ValueOf(v), buf)
}

func appendMarshal(v reflect.Value, buf []byte) []byte {
	if v == nilValue {
		return append(buf, "null"...)
	}
	kind := v.Kind()
	marshalFunc, ok := marshalFuncByKind[kind]
	if !ok {
		panic(fmt.Errorf("unsupported type: %s", v.Type()))
	}
	return marshalFunc(v, buf)
}

func cacheMarshalFunc(t reflect.Type, marshalFunc marshalFunc) {
	marshalFuncsByType.Store(t, marshalFunc)
}

func appendWithCachedFunc(v reflect.Value, buf []byte) (res []byte, ok bool) {
	marshalFunc, ok := marshalFuncsByType.Load(v.Type())
	if ok {
		return marshalFunc(v, buf), true
	}
	return nil, false
}

func appendBool(v reflect.Value, buf []byte) []byte {
	return strconv.AppendBool(buf, v.Bool())
}

func appendInt(v reflect.Value, buf []byte) []byte {
	return strconv.AppendInt(buf, v.Int(), 10)
}

func appendUint(v reflect.Value, buf []byte) []byte {
	return strconv.AppendUint(buf, v.Uint(), 10)
}

func appendFloat(v reflect.Value, buf []byte) []byte {
	return strconv.AppendFloat(buf, v.Float(), 'f', -1, 64)
}

func appendWithCustomMarshaler(v reflect.Value, buf []byte) (res []byte, ok bool) {
	switch vv := v.Interface().(type) {
	case Marshaler:
		cacheMarshalFunc(v.Type(), appendWithMarshalTo)
		return vv.MarshalJSONTo(buf), true
	case fmt.Stringer:
		cacheMarshalFunc(v.Type(), appendStringer)
		return AppendString(buf, vv.String()), true
	case stdJSON.Marshaler:
		cacheMarshalFunc(v.Type(), appendStdJSONMarshaler)
		return append(buf, must(vv.MarshalJSON())...), true
	case encoding.BinaryAppender:
		cacheMarshalFunc(v.Type(), appendBinaryAppender)
		//FIXME: append escaped
		return must(vv.AppendBinary(buf)), true
	case encoding.TextAppender:
		cacheMarshalFunc(v.Type(), appendTextAppender)
		//FIXME: append escaped
		return must(vv.AppendText(buf)), true
	case encoding.TextMarshaler:
		cacheMarshalFunc(v.Type(), appendTestMarshaler)
		return AppendString(buf, must(vv.MarshalText())), true
	case encoding.BinaryMarshaler:
		cacheMarshalFunc(v.Type(), appendBinaryMarshaler)
		return AppendString(buf, must(vv.MarshalBinary())), true
	}
	return nil, false
}

func mustAppendWithCustomMarshaler(v reflect.Value, buf []byte) []byte {
	res, ok := appendWithCustomMarshaler(v, buf)
	if !ok {
		panic(fmt.Errorf("tag %q used but no marshaler implemented: %s", tagUseMarshaler, v.Type()))
	}
	return res
}

func appendKV(k reflect.Value, v reflect.Value, buf []byte) []byte {
	buf = AppendString(buf, k.String())
	buf = append(buf, ':')
	return appendMarshal(v, buf)
}

func appendMap(v reflect.Value, buf []byte) []byte {
	if v.Type().Key().Kind() != reflect.String {
		panic(fmt.Errorf("map key must be string: %s", v.Type()))
	}
	buf = append(buf, '{')
	i := 0
	oldN := len(buf)
	iter := v.MapRange()
	for iter.Next() {
		k := iter.Key()
		v := iter.Value()
		buf = appendKV(k, v, buf)
		buf = append(buf, ',')
		i++
	}
	n := len(buf)
	if oldN != n {
		buf = buf[:n-1]
	}
	return append(buf, '}')
}

func appendArray(v reflect.Value, buf []byte) []byte {
	switch v.Type().Elem().Kind() {
	case reflect.String:
		return appendStringSlice(v, buf)
	case reflect.Uint8: // byte
		return appendBytesAsBase64(v, buf)
	}
	buf = append(buf, '[')
	oldN := len(buf)
	for i := range v.Len() {
		buf = appendMarshal(v.Index(i), buf)
		buf = append(buf, ',')
	}
	n := len(buf)
	if oldN != n {
		buf = buf[:n-1]
	}
	return append(buf, ']')
}

func appendPtrInterface(v reflect.Value, buf []byte) []byte {
	return appendMarshal(v.Elem(), buf)
}

func appendWithMarshalTo(v reflect.Value, buf []byte) []byte {
	return v.Interface().(Marshaler).MarshalJSONTo(buf)
}

func appendStringer(v reflect.Value, buf []byte) []byte {
	return AppendString(buf, v.Interface().(fmt.Stringer).String())
}

func appendStdJSONMarshaler(v reflect.Value, buf []byte) []byte {
	return append(buf, must(v.Interface().(stdJSON.Marshaler).MarshalJSON())...)
}

func appendBinaryAppender(v reflect.Value, buf []byte) []byte {
	//FIXME: append escaped
	return must(v.Interface().(encoding.BinaryAppender).AppendBinary(buf))
}

func appendTextAppender(v reflect.Value, buf []byte) []byte {
	//FIXME: append escaped
	return must(v.Interface().(encoding.TextAppender).AppendText(buf))
}

func appendTestMarshaler(v reflect.Value, buf []byte) []byte {
	return AppendString(buf, must(v.Interface().(encoding.TextMarshaler).MarshalText()))
}

func appendBinaryMarshaler(v reflect.Value, buf []byte) []byte {
	return AppendString(buf, must(v.Interface().(encoding.BinaryMarshaler).MarshalBinary()))
}
