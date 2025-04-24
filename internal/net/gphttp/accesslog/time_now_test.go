package accesslog

import (
	"testing"
	"time"
)

func BenchmarkTimeNow(b *testing.B) {
	b.Run("default", func(b *testing.B) {
		for b.Loop() {
			time.Now()
		}
	})

	b.Run("reduced_call", func(b *testing.B) {
		for b.Loop() {
			DefaultTimeNow()
		}
	})
}

func TestDefaultTimeNow(t *testing.T) {
	// Get initial time
	t1 := DefaultTimeNow()

	// Second call should return the same time without calling time.Now
	t2 := DefaultTimeNow()

	if !t1.Equal(t2) {
		t.Errorf("Expected t1 == t2, got t1 = %v, t2 = %v", t1, t2)
	}

	// Set shouldCallTimeNow to true
	shouldCallTimeNow.Store(true)

	// This should update the lastTimeNow
	t3 := DefaultTimeNow()

	// The time should have changed
	if t2.Equal(t3) {
		t.Errorf("Expected t2 != t3, got t2 = %v, t3 = %v", t2, t3)
	}

	// Fourth call should return the same time as third call
	t4 := DefaultTimeNow()

	if !t3.Equal(t4) {
		t.Errorf("Expected t3 == t4, got t3 = %v, t4 = %v", t3, t4)
	}
}

func TestMockTimeNow(t *testing.T) {
	// Save the original TimeNow function to restore later
	originalTimeNow := TimeNow
	defer func() {
		TimeNow = originalTimeNow
	}()

	// Create a fixed time
	fixedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)

	// Mock the time
	MockTimeNow(fixedTime)

	// TimeNow should return the fixed time
	result := TimeNow()

	if !result.Equal(fixedTime) {
		t.Errorf("Expected %v, got %v", fixedTime, result)
	}
}

func TestTimeNowTicker(t *testing.T) {
	// This test verifies that the ticker properly updates shouldCallTimeNow

	// Reset the flag
	shouldCallTimeNow.Store(false)

	// Wait for the ticker to tick (slightly more than the interval)
	time.Sleep(shouldCallTimeNowInterval + 10*time.Millisecond)

	// The ticker should have set shouldCallTimeNow to true
	if !shouldCallTimeNow.Load() {
		t.Error("Expected shouldCallTimeNow to be true after ticker interval")
	}

	// Call DefaultTimeNow which should reset the flag
	DefaultTimeNow()

	// Check that the flag is reset
	if shouldCallTimeNow.Load() {
		t.Error("Expected shouldCallTimeNow to be false after calling DefaultTimeNow")
	}
}

/*
BenchmarkTimeNow
BenchmarkTimeNow/default
BenchmarkTimeNow/default-20         	48158628	        24.86 ns/op	       0 B/op	       0 allocs/op
BenchmarkTimeNow/reduced_call
BenchmarkTimeNow/reduced_call-20    	1000000000	         1.000 ns/op	       0 B/op	       0 allocs/op
*/
