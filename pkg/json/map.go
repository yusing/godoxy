package json

import (
	"reflect"
)

type Map[V any] map[string]V

func (m Map[V]) MarshalJSONTo(buf []byte) []byte {
	oldN := len(buf)
	buf = append(buf, '{')
	for k, v := range m {
		buf = AppendString(buf, k)
		buf = append(buf, ':')
		buf = appendMarshal(reflect.ValueOf(v), buf)
		buf = append(buf, ',')
	}
	n := len(buf)
	if oldN != n {
		buf = buf[:n-1]
	}
	return append(buf, '}')
}
