package utils

import (
	"time"

	"go.uber.org/atomic"
)

var (
	TimeNow           = DefaultTimeNow
	shouldCallTimeNow atomic.Bool
	timeNowTicker     = time.NewTicker(shouldCallTimeNowInterval)
	lastTimeNow       = atomic.NewTime(time.Now())
)

const shouldCallTimeNowInterval = 100 * time.Millisecond

func MockTimeNow(t time.Time) {
	TimeNow = func() time.Time {
		return t
	}
}

// DefaultTimeNow is a time.Now wrapper that reduces the number of calls to time.Now
// by caching the result and only allow calling time.Now when the ticker fires.
//
// Returned value may have +-100ms error.
func DefaultTimeNow() time.Time {
	swapped := shouldCallTimeNow.CompareAndSwap(false, true)
	if swapped { // first call
		now := time.Now()
		lastTimeNow.Store(now)
		return now
	}
	return lastTimeNow.Load()
}

func init() {
	go func() {
		for range timeNowTicker.C {
			shouldCallTimeNow.Store(true)
		}
	}()
}
