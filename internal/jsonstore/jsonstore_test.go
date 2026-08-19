package jsonstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTest(t *testing.T) {
	prevStoresPath := storesPath
	storesPath = t.TempDir()
	t.Cleanup(func() {
		storesPath = prevStoresPath
		clear(stores)
		clear(lastSaved)
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

func TestSaveSkipsUnchangedStore(t *testing.T) {
	setupTest(t)

	store := Store[string]("test")
	store.Store("a", "1")
	if err := save(); err != nil {
		t.Fatal(err)
	}

	// a second save with no change must not rewrite the file
	path := filepath.Join(storesPath, "test.json")
	if err := os.WriteFile(path, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sentinel" {
		t.Fatalf("expected unchanged store to be skipped, got %q", data)
	}

	store.Store("b", "2")
	if err := save(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "sentinel" {
		t.Fatal("expected changed store to be written")
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	setupTest(t)

	store := Store[string]("test")
	store.Store("a", "1")
	if err := save(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(storesPath, "test.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected no temp file, got err=%v", err)
	}
}

func TestSaveEveryFlushesWithoutShutdown(t *testing.T) {
	setupTest(t)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	saverDone := make(chan struct{})
	go func() {
		defer close(saverDone)
		saveEvery(ctx, time.Millisecond)
	}()

	store := Store[string]("test")
	store.Store("a", "1")

	path := filepath.Join(storesPath, "test.json")
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("store was not flushed periodically")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()
	<-saverDone

	clear(stores)
	loaded := Store[string]("test")
	if v, ok := loaded.Load("a"); !ok || v != "1" {
		t.Fatalf("expected a=1 after periodic flush, got %q ok=%v", v, ok)
	}
}
