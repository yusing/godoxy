package json

import (
	"reflect"

	"github.com/bytedance/sonic"
	"github.com/yusing/go-proxy/internal/utils/synk"
)

type Marshaler interface {
	MarshalJSONTo(buf []byte) []byte
}

var (
	Unmarshal  = sonic.Unmarshal
	Valid      = sonic.Valid
	NewDecoder = sonic.ConfigDefault.NewDecoder
)

// Marshal returns the JSON encoding of v.
//
// It's like json.Marshal, but with some differences:
//
//   - It supports custom Marshaler interface (MarshalJSONTo(buf []byte) []byte)
//     to allow further optimizations.
//
//   - It leverages the strutils library.
//
//   - It drops the need to implement Marshaler or json.Marshaler by supports extra field tags:
//
//     `byte_size` to format the field to human readable size.
//
//     `unix_time` to format the uint64 field to string date-time without specifying MarshalJSONTo.
//
//     `use_marshaler` to force using the custom marshaler for primitive types declaration (e.g. `type Status int`).
//
//   - It corrects the behavior of *url.URL and time.Duration.
//
//   - It does not support maps other than string-keyed maps.
func Marshal(v any) ([]byte, error) {
	buf := bytesPool.Get()
	defer bytesPool.Put(buf)
	return cloneBytes(appendMarshal(reflect.ValueOf(v), buf)), nil
}

func MarshalTo(v any, buf []byte) []byte {
	return appendMarshal(reflect.ValueOf(v), buf)
}

const initBufSize = 4096

var bytesPool = synk.NewBytesPool(initBufSize, synk.DefaultMaxBytes)

func cloneBytes(buf []byte) (res []byte) {
	return append(res, buf...)
}
