package jsonstore

import (
	"testing"
)

func setupTest(t *testing.T) {
	prevStoresPath := storesPath
	storesPath = t.TempDir()
	t.Cleanup(func() {
		storesPath = prevStoresPath
		clear(stores)
	})
}

func TestNewJSON(t *testing.T) {
	setupTest(t)

	store := Store[string]("test")
	store.Store("a", "1")
	if v, _ := store.Load("a"); v != "1" {
		t.Fatal("expected 1, got", v)
	}
}

func TestSaveLoadStore(t *testing.T) {
	setupTest(t)

	store := Store[string]("test")
	store.Store("a", "1")
	if err := save(); err != nil {
		t.Fatal(err)
	}
	// reload
	clear(stores)
	loaded := Store[string]("test")
	v, ok := loaded.Load("a")
	if !ok {
		t.Fatal("expected key exists")
	}
	if v != "1" {
		t.Fatalf("expected 1, got %q", v)
	}
	if loaded.Map == store.Map {
		t.Fatal("expected different objects")
	}
}

type testObject struct {
	I int    `json:"i"`
	S string `json:"s"`
}

func (*testObject) Initialize() {}

func TestSaveLoadObject(t *testing.T) {
	setupTest(t)

	obj := Object[*testObject]("test")
	obj.I = 1
	obj.S = "1"
	if err := save(); err != nil {
		t.Fatal(err)
	}
	// reload
	clear(stores)
	loaded := Object[*testObject]("test")
	if loaded.I != 1 || loaded.S != "1" {
		t.Fatalf("expected 1, got %d, %s", loaded.I, loaded.S)
	}
	if loaded == obj {
		t.Fatal("expected different objects")
	}
}
