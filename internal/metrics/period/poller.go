package period

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/utils/atomic"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type (
	PollFunc[T any]                      func(ctx context.Context, lastResult T) (T, error)
	AggregateFunc[T any, AggregateT any] func(entries []T, query url.Values) (total int, result AggregateT)
	FilterFunc[T any]                    func(entries []T, keyword string) (filtered []T)
	Poller[T any, AggregateT any]        struct {
		name         string
		poll         PollFunc[T]
		aggregate    AggregateFunc[T, AggregateT]
		resultFilter FilterFunc[T]
		period       *Period[T]
		lastResult   atomic.Value[T]
		errs         []pollErr
	}
	pollErr struct {
		err   error
		count int
	}
)

const (
	PollInterval       = 1 * time.Second
	gatherErrsInterval = 30 * time.Second
	saveInterval       = 5 * time.Minute

	gatherErrsTicks = int(gatherErrsInterval / PollInterval) // 30
	saveTicks       = int(saveInterval / PollInterval)       // 300

	saveBaseDir = "data/metrics"
)

var initDataDirOnce sync.Once

func initDataDir() {
	if err := os.MkdirAll(saveBaseDir, 0o755); err != nil {
		log.Error().Err(err).Msg("failed to create metrics data directory")
	}
}

func NewPoller[T any, AggregateT any](
	name string,
	poll PollFunc[T],
	aggregator AggregateFunc[T, AggregateT],
) *Poller[T, AggregateT] {
	return &Poller[T, AggregateT]{
		name:      name,
		poll:      poll,
		aggregate: aggregator,
		period:    NewPeriod[T](),
	}
}

func (p *Poller[T, AggregateT]) savePath() string {
	return filepath.Join(saveBaseDir, p.name+".json")
}

func (p *Poller[T, AggregateT]) load() error {
	entries, err := os.ReadFile(p.savePath())
	if err != nil {
		return err
	}
	if err := sonic.Unmarshal(entries, &p.period); err != nil {
		return err
	}
	// Validate and fix intervals after loading to ensure data integrity.
	p.period.ValidateAndFixIntervals()
	return nil
}

func (p *Poller[T, AggregateT]) save() error {
	initDataDirOnce.Do(initDataDir)
	entries, err := sonic.Marshal(p.period)
	if err != nil {
		return err
	}
	return os.WriteFile(p.savePath(), entries, 0o644)
}

func (p *Poller[T, AggregateT]) WithResultFilter(filter FilterFunc[T]) *Poller[T, AggregateT] {
	p.resultFilter = filter
	return p
}

func (p *Poller[T, AggregateT]) appendErr(err error) {
	if len(p.errs) == 0 {
		p.errs = []pollErr{
			{err: err, count: 1},
		}
		return
	}
	for i, e := range p.errs {
		if e.err.Error() == err.Error() {
			p.errs[i].count++
			return
		}
	}
	p.errs = append(p.errs, pollErr{err: err, count: 1})
}

func (p *Poller[T, AggregateT]) gatherErrs() (string, bool) {
	if len(p.errs) == 0 {
		return "", false
	}
	errs := gperr.NewBuilder(fmt.Sprintf("poller %s has encountered %d errors in the last %s:", p.name, len(p.errs), gatherErrsInterval))
	for _, e := range p.errs {
		errs.Addf("%w: %d times", e.err, e.count)
	}
	return errs.String(), true
}

func (p *Poller[T, AggregateT]) clearErrs() {
	p.errs = p.errs[:0]
}

func (p *Poller[T, AggregateT]) pollWithTimeout(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, PollInterval)
	defer cancel()
	data, err := p.poll(ctx, p.lastResult.Load())
	if err != nil {
		p.appendErr(err)
		return
	}
	p.period.Add(data)
	p.lastResult.Store(data)
}

func (p *Poller[T, AggregateT]) Start() {
	t := task.RootTask("poller."+p.name, true)
	l := log.With().Str("name", p.name).Logger()
	err := p.load()
	if err != nil {
		if !os.IsNotExist(err) {
			l.Err(err).Msg("failed to load last metrics data")
		}
	} else {
		l.Debug().Int("entries", p.period.Total()).Msgf("Loaded last metrics data")
	}

	go func() {
		ticker := time.NewTicker(PollInterval)
		defer ticker.Stop()

		var tickCount int

		defer func() {
			err := p.save()
			if err != nil {
				l.Err(err).Msg("failed to save metrics data")
			}
			t.Finish(err)
		}()

		l.Debug().Dur("interval", PollInterval).Msg("Starting poller")

		p.pollWithTimeout(t.Context())

		for {
			select {
			case <-t.Context().Done():
				return
			case <-ticker.C:
				p.pollWithTimeout(t.Context())

				tickCount++

				if tickCount%gatherErrsTicks == 0 {
					errs, ok := p.gatherErrs()
					if ok {
						log.Error().Msg(errs)
					}
					p.clearErrs()
				}

				if tickCount%saveTicks == 0 {
					err := p.save()
					if err != nil {
						p.appendErr(err)
					}
				}
			}
		}
	}()
}

func (p *Poller[T, AggregateT]) Get(filter Filter) ([]T, bool) {
	return p.period.Get(filter)
}

func (p *Poller[T, AggregateT]) GetLastResult() T {
	return p.lastResult.Load()
}
