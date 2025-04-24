package jsonstore

import (
	"testing"
)

func TestNewJSON(t *testing.T) {
	store := Store[string]("test")
	store.Store("a", "1")
	if v, _ := store.Load("a"); v != "1" {
		t.Fatal("expected 1, got", v)
	}
}

func TestSaveLoad(t *testing.T) {
	storesPath = t.TempDir()
	store := Store[string]("test")
	store.Store("a", "1")
	if err := save(); err != nil {
		t.Fatal(err)
	}
	stores.m = nil
	if err := load(); err != nil {
		t.Fatal(err)
	}
	store = Store[string]("test")
	if v, _ := store.Load("a"); v != "1" {
		t.Fatal("expected 1, got", v)
	}
}
