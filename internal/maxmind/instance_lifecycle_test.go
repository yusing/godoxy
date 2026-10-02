package maxmind

import (
	"errors"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/yusing/godoxy/internal/common"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
)

func waitMaxMindCollected[T any](t *testing.T, ref weak.Pointer[T]) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if ref.Value() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("MaxMind runtime or its configuration remains strongly retained")
}

func newWeakMaxMind(t *testing.T, parent task.Parent, cfg *Config) weak.Pointer[MaxMind] {
	t.Helper()
	instance, err := New(parent, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// Populate the cache, including its error path, before dropping the runtime.
	_, err = instance.lookupCity(t.Context(), "not-an-ip")
	if cfg.Database == "" {
		if !errors.Is(err, ErrDBNotLoaded) {
			t.Fatalf("lookup error = %v, want ErrDBNotLoaded", err)
		}
	} else if !errors.Is(err, ErrInvalidIP) {
		t.Fatalf("lookup error = %v, want ErrInvalidIP", err)
	}
	return weak.Make(instance)
}

func TestNewMaxMindRuntimeCollectedAfterCancellation(t *testing.T) {
	oldIsTest := common.IsTest
	common.IsTest = true
	t.Cleanup(func() { common.IsTest = oldIsTest })
	mockMaxMindDBOpen(t)
	for _, database := range []maxmind.DatabaseType{"", maxmind.MaxMindGeoLite} {
		t.Run("database="+string(database), func(t *testing.T) {
			parent := task.RootTask("maxmind_lifecycle", true)
			t.Cleanup(func() { parent.Finish(nil) })
			ref := newWeakMaxMind(t, parent, &Config{Database: database})
			runtime.GC()
			if ref.Value() == nil {
				t.Fatal("active cache unexpectedly lost its runtime")
			}
			parent.Finish(nil)
			waitMaxMindCollected(t, ref)
			// Keep the owner itself alive: cancellation must release the runtime,
			// rather than relying on collection of the owner's task tree.
			runtime.KeepAlive(parent)
		})
	}
}

func newFailedWeakMaxMindConfig(t *testing.T, parent task.Parent) weak.Pointer[Config] {
	t.Helper()
	cfg := &Config{Database: maxmind.MaxMindGeoLite}
	instance, err := New(parent, cfg)
	if instance != nil || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("New = %v, %v; want nil and permission error", instance, err)
	}
	// New returns no runtime on failure. Its configuration is an observable
	// retention sentinel because any leaked lookup closure owns the runtime
	// and therefore this exact configuration.
	return weak.Make(cfg)
}

func TestNewMaxMindInitializationFailureDoesNotRetainRuntime(t *testing.T) {
	oldOpen := maxmindDBOpen
	maxmindDBOpen = func(string, ...maxminddb.ReaderOption) (*maxminddb.Reader, error) {
		return nil, os.ErrPermission
	}
	t.Cleanup(func() { maxmindDBOpen = oldOpen })
	parent := task.RootTask("maxmind_failed_initialization", true)
	t.Cleanup(func() { parent.Finish(nil) })
	ref := newFailedWeakMaxMindConfig(t, parent)
	// Do not cancel first: failed initialization must never register a cache
	// that retains the failed runtime until an otherwise unrelated shutdown.
	waitMaxMindCollected(t, ref)
	runtime.KeepAlive(parent)
}

func TestNewMaxMindCancellationDuringInitializationDoesNotRetainRuntime(t *testing.T) {
	oldIsTest := common.IsTest
	common.IsTest = true
	t.Cleanup(func() { common.IsTest = oldIsTest })
	entered := make(chan struct{})
	resume := make(chan struct{})
	var unblock sync.Once
	oldOpen := maxmindDBOpen
	maxmindDBOpen = func(string, ...maxminddb.ReaderOption) (*maxminddb.Reader, error) {
		close(entered)
		<-resume
		return maxminddb.Open("testdata/GeoIP2-City-Test.mmdb")
	}
	t.Cleanup(func() { maxmindDBOpen = oldOpen })
	parent := task.RootTask("maxmind_cancel_during_initialization", true)
	t.Cleanup(func() { parent.Finish(nil) })
	canceled := make(chan struct{})
	parent.OnCancel("observe initialization cancellation", func() { close(canceled) })
	type result struct {
		ref weak.Pointer[MaxMind]
		err error
	}
	results := make(chan result, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		instance, err := New(parent, &Config{Database: maxmind.MaxMindGeoLite})
		results <- result{ref: weak.Make(instance), err: err}
	}()
	t.Cleanup(func() {
		unblock.Do(func() { close(resume) })
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("initialization goroutine did not finish")
		}
	})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("database initialization did not start")
	}
	parent.Finish(nil)
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("parent cancellation callbacks did not run")
	}
	// Resume only after cancellation callbacks have run. The cache is built
	// afterward and must not outlive its already-canceled owner.
	unblock.Do(func() { close(resume) })
	var got result
	select {
	case got = <-results:
	case <-time.After(3 * time.Second):
		t.Fatal("New did not complete after initialization resumed")
	}
	if got.err != nil {
		t.Fatalf("New after cancellation = %v", got.err)
	}
	waitMaxMindCollected(t, got.ref)
	runtime.KeepAlive(parent)
}
