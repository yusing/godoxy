package utils

import (
	"time"

	"go.uber.org/atomic"
)

var (
	TimeNow           = DefaultTimeNow
	shouldCallTimeNow atomic.Bool
	timeNowTicker     = time.NewTicker(shouldCallTimeNowInterval)
	lastTimeNow       = time.Now()
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
	if shouldCallTimeNow.Load() {
		lastTimeNow = time.Now()
		shouldCallTimeNow.Store(false)
	}
	return lastTimeNow
}

func init() {
	go func() {
		for range timeNowTicker.C {
			shouldCallTimeNow.Store(true)
		}
	}()
}
