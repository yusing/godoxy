package period

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
)

type Entries[T any] struct {
	entries  [maxEntries]T
	index    int
	count    int
	interval time.Duration
	lastAdd  time.Time
}

const maxEntries = 100

func newEntries[T any](duration time.Duration) *Entries[T] {
	interval := max(duration/maxEntries, time.Second)
	return &Entries[T]{
		interval: interval,
		lastAdd:  time.Now(),
	}
}

func (e *Entries[T]) Add(now time.Time, info T) {
	if now.Sub(e.lastAdd) < e.interval {
		return
	}
	e.addWithTime(now, info)
}

// addWithTime adds an entry with a specific timestamp without interval checking.
// This is used internally for reconstructing historical data.
func (e *Entries[T]) addWithTime(timestamp time.Time, info T) {
	e.entries[e.index] = info
	e.index = (e.index + 1) % maxEntries
	if e.count < maxEntries {
		e.count++
	}
	e.lastAdd = timestamp
}

// validateInterval checks if the current interval matches the expected interval for the duration.
// Returns true if valid, false if the interval needs to be recalculated.
func (e *Entries[T]) validateInterval(expectedDuration time.Duration) bool {
	expectedInterval := max(expectedDuration/maxEntries, time.Second)
	return e.interval == expectedInterval
}

// fixInterval recalculates and sets the correct interval based on the expected duration.
func (e *Entries[T]) fixInterval(expectedDuration time.Duration) {
	e.interval = max(expectedDuration/maxEntries, time.Second)
}

func (e *Entries[T]) Get() []T {
	if e.count < maxEntries {
		return e.entries[:e.count]
	}
	var res [maxEntries]T
	if e.index >= e.count {
		copy(res[:], e.entries[:e.count])
	} else {
		copy(res[:], e.entries[e.index:])
		copy(res[e.count-e.index:], e.entries[:e.index])
	}
	return res[:]
}

type entriesJSON[T any] struct {
	Entries  []T           `json:"entries"`
	Interval time.Duration `json:"interval"`
}

func (e *Entries[T]) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(entriesJSON[T]{
		Entries:  e.Get(),
		Interval: e.interval,
	})
}

func (e *Entries[T]) UnmarshalJSON(data []byte) error {
	var v entriesJSON[T]
	v.Entries = make([]T, 0, maxEntries)
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	if len(v.Entries) == 0 {
		return nil
	}
	entries := v.Entries
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}

	// Set the interval first before adding entries.
	e.interval = v.Interval

	// Add entries with proper time spacing to respect the interval.
	now := time.Now()
	for i, info := range entries {
		// Calculate timestamp based on entry position and interval.
		// Most recent entry gets current time, older entries get earlier times.
		entryTime := now.Add(-time.Duration(len(entries)-1-i) * e.interval)
		e.addWithTime(entryTime, info)
	}
	return nil
}
