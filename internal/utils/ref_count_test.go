package utils

import (
	"sync"
	"testing"
	"time"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRefCounterAddSub(t *testing.T) {
	rc := NewRefCounter() // Count starts at 1

	var wg sync.WaitGroup

	rc.Add()
	for range 2 {
		wg.Go(func() {
			rc.Sub()
		})
	}

	wg.Wait()
	ExpectEqual(t, int(rc.refCount), 0)

	select {
	case <-rc.Zero():
		// Expected behavior
	case <-time.After(1 * time.Second):
		t.Fatal("Expected Zero channel to close, but it didn't")
	}
}

func TestRefCounterMultipleAddSub(t *testing.T) {
	rc := NewRefCounter()

	var wg sync.WaitGroup
	numAdds := 5
	numSubs := 5
	wg.Add(numAdds)

	for range numAdds {
		go func() {
			defer wg.Done()
			rc.Add()
		}()
	}
	wg.Wait()
	ExpectEqual(t, int(rc.refCount), numAdds+1)

	wg.Add(numSubs)
	for range numSubs {
		go func() {
			defer wg.Done()
			rc.Sub()
		}()
	}
	wg.Wait()
	ExpectEqual(t, int(rc.refCount), numAdds+1-numSubs)

	rc.Sub()
	select {
	case <-rc.Zero():
		// Expected behavior
	case <-time.After(1 * time.Second):
		t.Fatal("Expected Zero channel to close, but it didn't")
	}
}

func TestRefCounterOneInitially(t *testing.T) {
	rc := NewRefCounter()
	rc.Sub() // Bring count to zero

	select {
	case <-rc.Zero():
		// Expected behavior
	case <-time.After(1 * time.Second):
		t.Fatal("Expected Zero channel to close, but it didn't")
	}
}
