package json

type MapSlice[V any] []Map[V]

func (s MapSlice[V]) MarshalJSONTo(buf []byte) []byte {
	buf = append(buf, '[')
	i := 0
	n := len(s)
	for _, entry := range s {
		buf = entry.MarshalJSONTo(buf)
		if i != n-1 {
			buf = append(buf, ',')
		}
		i++
	}
	buf = append(buf, ']')
	return buf
}
