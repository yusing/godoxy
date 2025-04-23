package trie

import (
	"maps"

	"github.com/bytedance/sonic"
)

var sonicConfig = sonic.Config{
	EncodeNullForInfOrNan: true,
}.Froze()

func (r *Root) MarshalJSON() ([]byte, error) {
	return sonicConfig.Marshal(maps.Collect(r.Walk))
}

func (r *Root) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := sonicConfig.Unmarshal(data, &m); err != nil {
		return err
	}
	for k, v := range m {
		r.Store(NewKey(k), v)
	}
	return nil
}
