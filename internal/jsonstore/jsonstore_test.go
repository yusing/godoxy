package jsonstore

import (
	"path/filepath"
	"testing"
)

func TestNewJSON(t *testing.T) {
	store := NewStore[string]("test")
	store.Store("a", "1")
	if v, _ := store.Load("a"); v != "1" {
		t.Fatal("expected 1, got", v)
	}
}

func TestSaveLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storesPath = filepath.Join(tmpDir, "data.json")
	store := NewStore[string]("test")
	store.Store("a", "1")
	if err := save(); err != nil {
		t.Fatal(err)
	}
	stores = nil
	if err := load(); err != nil {
		t.Fatal(err)
	}
	store = NewStore[string]("test")
	if v, _ := store.Load("a"); v != "1" {
		t.Fatal("expected 1, got", v)
	}
}
