package period

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/go-proxy/pkg/json"
)

func (p *Poller[T, AggregateT]) Test(t *testing.T, query url.Values) {
	t.Helper()
	for range 3 {
		require.NoError(t, p.testPoll())
	}
	t.Run("periods", func(t *testing.T) {
		assert.NoError(t, p.testMarshalPeriods(query))
	})
	t.Run("no period", func(t *testing.T) {
		assert.NoError(t, p.testMarshalNoPeriod())
	})
}

func (p *Poller[T, AggregateT]) testPeriod(period string, query url.Values) (any, error) {
	query.Set("period", period)
	return p.getRespData(&http.Request{URL: &url.URL{RawQuery: query.Encode()}})
}

func (p *Poller[T, AggregateT]) testPoll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	data, err := p.poll(ctx, p.lastResult.Load())
	if err != nil {
		return err
	}
	for _, period := range p.period.Entries {
		period.Add(time.Now(), data)
	}
	p.lastResult.Store(data)
	return nil
}

func (p *Poller[T, AggregateT]) testMarshalPeriods(query url.Values) error {
	for period := range p.period.Entries {
		data, err := p.testPeriod(string(period), query)
		if err != nil {
			return err
		}
		_, err = json.Marshal(data)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Poller[T, AggregateT]) testMarshalNoPeriod() error {
	data, err := p.getRespData(&http.Request{URL: &url.URL{}})
	if err != nil {
		return err
	}
	_, err = json.Marshal(data)
	if err != nil {
		return err
	}
	return nil
}
