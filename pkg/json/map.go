package json

import (
	"reflect"
)

type Map[V any] map[string]V

func (m Map[V]) MarshalJSONTo(buf []byte) []byte {
	buf = append(buf, '{')
	i := 0
	n := len(m)
	for k, v := range m {
		buf = AppendString(buf, k)
		buf = append(buf, ':')
		buf = appendMarshal(reflect.ValueOf(v), buf)
		if i != n-1 {
			buf = append(buf, ',')
		}
		i++
	}
	buf = append(buf, '}')
	return buf
}
