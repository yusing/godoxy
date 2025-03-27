package period

import (
	"sync"
	"time"
)

type Period[T any] struct {
	Entries map[Filter]*Entries[T] `json:"entries"`
	mu      sync.RWMutex
}

type Filter string

func NewPeriod[T any]() *Period[T] {
	return &Period[T]{
		Entries: map[Filter]*Entries[T]{
			"5m":  newEntries[T](5 * time.Minute),
			"15m": newEntries[T](15 * time.Minute),
			"1h":  newEntries[T](1 * time.Hour),
			"1d":  newEntries[T](24 * time.Hour),
			"1mo": newEntries[T](30 * 24 * time.Hour),
		},
	}
}

func (p *Period[T]) Add(info *T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, period := range p.Entries {
		period.Add(now, info)
	}
}

func (p *Period[T]) Get(filter Filter) ([]*T, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	period, ok := p.Entries[filter]
	if !ok {
		return nil, false
	}
	return period.Get(), true
}

func (p *Period[T]) Total() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	total := 0
	for _, period := range p.Entries {
		total += period.count
	}
	return total
}
