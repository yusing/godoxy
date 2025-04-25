package trie

import (
	"encoding/json"
	"testing"
)

func TestMarshalUnmarshalJSON(t *testing.T) {
	trie := NewTrie()
	data := map[string]any{
		"foo.bar":      42.12,
		"foo.baz":      "hello",
		"qwe.rt.yu.io": 123.45,
	}
	for k, v := range data {
		trie.Store(NewKey(k), v)
	}

	// MarshalJSON
	bytesFromTrie, err := json.Marshal(trie)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	// UnmarshalJSON
	newTrie := NewTrie()
	if err := json.Unmarshal(bytesFromTrie, newTrie); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	for k, v := range data {
		got, ok := newTrie.Get(NewKey(k))
		if !ok || got != v {
			t.Errorf("UnmarshalJSON: key %q got %v, want %v", k, got, v)
		}
	}
}
