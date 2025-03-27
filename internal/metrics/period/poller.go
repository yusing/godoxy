package period

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/atomic"
)

type (
	PollFunc[T any]                                 func(ctx context.Context, lastResult *T) (*T, error)
	AggregateFunc[T any, AggregateT json.Marshaler] func(entries []*T, query url.Values) (total int, result AggregateT)
	FilterFunc[T any]                               func(entries []*T, keyword string) (filtered []*T)
	Poller[T any, AggregateT json.Marshaler]        struct {
		name         string
		poll         PollFunc[T]
		aggregate    AggregateFunc[T, AggregateT]
		resultFilter FilterFunc[T]
		period       *Period[T]
		lastResult   atomic.Value[*T]
		errs         []pollErr
	}
	pollErr struct {
		err   error
		count int
	}
)

const (
	pollInterval       = 1 * time.Second
	gatherErrsInterval = 30 * time.Second
	saveInterval       = 5 * time.Minute

	saveBaseDir = "data/metrics"
)

func init() {
	if err := os.MkdirAll(saveBaseDir, 0o755); err != nil {
		panic(fmt.Sprintf("failed to create metrics data directory: %s", err))
	}
}

func NewPoller[T any, AggregateT json.Marshaler](
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
	return filepath.Join(saveBaseDir, fmt.Sprintf("%s.json", p.name))
}

func (p *Poller[T, AggregateT]) load() error {
	entries, err := os.ReadFile(p.savePath())
	if err != nil {
		return err
	}
	return json.Unmarshal(entries, &p.period)
}

func (p *Poller[T, AggregateT]) save() error {
	entries, err := json.Marshal(p.period)
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
	ctx, cancel := context.WithTimeout(ctx, pollInterval)
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
	t := task.RootTask("poller." + p.name)
	go func() {
		err := p.load()
		if err != nil {
			if !os.IsNotExist(err) {
				logging.Error().Err(err).Msgf("failed to load last metrics data for %s", p.name)
			}
		} else {
			logging.Debug().Msgf("Loaded last metrics data for %s, %d entries", p.name, p.period.Total())
		}

		pollTicker := time.NewTicker(pollInterval)
		gatherErrsTicker := time.NewTicker(gatherErrsInterval)
		saveTicker := time.NewTicker(saveInterval)

		defer func() {
			pollTicker.Stop()
			gatherErrsTicker.Stop()
			saveTicker.Stop()

			p.save()
			t.Finish(nil)
		}()

		logging.Debug().Msgf("Starting poller %s with interval %s", p.name, pollInterval)

		p.pollWithTimeout(t.Context())

		for {
			select {
			case <-t.Context().Done():
				return
			case <-pollTicker.C:
				p.pollWithTimeout(t.Context())
			case <-saveTicker.C:
				err := p.save()
				if err != nil {
					p.appendErr(err)
				}
			case <-gatherErrsTicker.C:
				errs, ok := p.gatherErrs()
				if ok {
					logging.Error().Msg(errs)
				}
				p.clearErrs()
			}
		}
	}()
}

func (p *Poller[T, AggregateT]) Get(filter Filter) ([]*T, bool) {
	return p.period.Get(filter)
}

func (p *Poller[T, AggregateT]) GetLastResult() *T {
	return p.lastResult.Load()
}
