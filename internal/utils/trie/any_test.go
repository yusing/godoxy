package trie

import (
	"testing"
)

func TestStoreNil(t *testing.T) {
	var v AnyValue
	v.Store(nil)
	if v.Load() != nil {
		t.Fatal("expected nil")
	}
	if !v.IsNil() {
		t.Fatal("expected true")
	}
}
