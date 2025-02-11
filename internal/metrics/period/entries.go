package period

import "time"

type Entries[T any] struct {
	entries  [maxEntries]*T
	index    int
	count    int
	interval int64
	lastAdd  int64
}

const maxEntries = 500

func newEntries[T any](interval int64) *Entries[T] {
	return &Entries[T]{
		interval: interval,
		lastAdd:  time.Now().Unix(),
	}
}

func (e *Entries[T]) Add(now int64, info *T) {
	if now-e.lastAdd < e.interval {
		return
	}
	e.entries[e.index] = info
	e.index++
	if e.index >= maxEntries {
		e.index = 0
	}
	if e.count < maxEntries {
		e.count++
	}
	e.lastAdd = now
}

func (e *Entries[T]) Get() []*T {
	if e.count < maxEntries {
		return e.entries[:e.count]
	}
	res := make([]*T, maxEntries)
	copy(res, e.entries[e.index:])
	copy(res[maxEntries-e.index:], e.entries[:e.index])
	return res
}
