package period

import (
	"sync"
	"time"
)

type Period[T any] struct {
	Entries map[Filter]*Entries[T] `json:"entries"`
	mu      sync.RWMutex
}

type Filter string // @name MetricsPeriod

const (
	MetricsPeriod5m  Filter = "5m"  // @name MetricsPeriod5m
	MetricsPeriod15m Filter = "15m" // @name MetricsPeriod15m
	MetricsPeriod1h  Filter = "1h"  // @name MetricsPeriod1h
	MetricsPeriod1d  Filter = "1d"  // @name MetricsPeriod1d
	MetricsPeriod1mo Filter = "1mo" // @name MetricsPeriod1mo
)

func NewPeriod[T any]() *Period[T] {
	return &Period[T]{
		Entries: map[Filter]*Entries[T]{
			MetricsPeriod5m:  newEntries[T](5 * time.Minute),
			MetricsPeriod15m: newEntries[T](15 * time.Minute),
			MetricsPeriod1h:  newEntries[T](1 * time.Hour),
			MetricsPeriod1d:  newEntries[T](24 * time.Hour),
			MetricsPeriod1mo: newEntries[T](30 * 24 * time.Hour),
		},
	}
}

func (p *Period[T]) Add(info T) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for _, period := range p.Entries {
		period.Add(now, info)
	}
}

func (p *Period[T]) Get(filter Filter) ([]T, bool) {
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
