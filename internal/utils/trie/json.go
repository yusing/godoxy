package trie

import (
	"encoding/json"
	"maps"
)

func (r *Root) MarshalJSON() ([]byte, error) {
	return json.Marshal(maps.Collect(r.Walk))
}

func (r *Root) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, v := range m {
		r.Store(NewKey(k), v)
	}
	return nil
}
