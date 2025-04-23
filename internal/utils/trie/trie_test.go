package trie

import "testing"

var nsCPU = Namespace("cpu")

// Test functions
func TestLoadOrStore(t *testing.T) {
	trie := NewTrie()
	ptr := trie.LoadOrStore(nsCPU, func() any {
		return new(int)
	})
	if ptr == nil {
		t.Fatal("expected pointer to be created")
	}
	if ptr != trie.LoadOrStore(nsCPU, func() any {
		return new(int)
	}) {
		t.Fatal("expected same pointer to be returned")
	}
	got, ok := trie.Get(nsCPU)
	if !ok || got != ptr {
		t.Fatal("expected same pointer to be returned")
	}
}

func TestStore(t *testing.T) {
	trie := NewTrie()
	ptr := new(int)
	trie.Store(nsCPU, ptr)
	got, ok := trie.Get(nsCPU)
	if !ok || got != ptr {
		t.Fatal("expected same pointer to be returned")
	}
}
