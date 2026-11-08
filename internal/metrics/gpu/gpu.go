// Package gpu collects per-engine GPU utilization.
package gpu

import (
	"errors"
	"sync"

	"github.com/rs/zerolog/log"
)

type Stat struct {
	Name    string             `json:"name"`
	Engines map[string]float64 `json:"engines"` // busy percent (0-100) by engine
} // @name GPUStat

// Collector reports utilization for all GPUs of one kind (e.g. Intel).
type Collector interface {
	Collect() ([]Stat, error)
}

// ErrNotFound is returned by a collector factory when no GPU of its kind is present.
var ErrNotFound = errors.New("no gpu found")

var (
	initOnce   sync.Once
	collectors []Collector
	collectMu  sync.Mutex
)

func initCollectors() {
	for kind, newCollector := range factories {
		c, err := newCollector()
		if err != nil && !errors.Is(err, ErrNotFound) {
			log.Warn().Err(err).Str("kind", kind).
				Msg("cannot read gpu counters, set GODOXY_METRICS_DISABLE_GPU=true to silence this")
		}
		if c != nil {
			collectors = append(collectors, c)
		}
	}
}

// Collect returns the current stats of every detected GPU.
// GPUs that are absent or not readable (e.g. missing permission) are skipped.
func Collect() []Stat {
	initOnce.Do(initCollectors)

	collectMu.Lock()
	defer collectMu.Unlock()

	var stats []Stat
	for _, c := range collectors {
		s, err := c.Collect()
		if err != nil {
			log.Debug().Err(err).Msg("failed to collect gpu stats")
		}
		stats = append(stats, s...)
	}
	return stats
}
