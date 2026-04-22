package maxmind

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestLookupCityPropagatesContext(t *testing.T) {
	oldLookupCityFn := lookupCityFn
	t.Cleanup(func() {
		lookupCityFn = oldLookupCityFn
	})

	type contextKey string
	const wantIP = "1.1.1.1"
	const wantValue contextKey = "trace-id"

	lookupCityFn = func(ctx context.Context, ipStr string) (*City, error) {
		require.Equal(t, wantIP, ipStr)
		require.Equal(t, "trace-123", ctx.Value(wantValue))
		return &City{}, nil
	}

	ctx := context.WithValue(t.Context(), wantValue, "trace-123")
	city, err := lookupCityFn(ctx, wantIP)
	require.NoError(t, err)
	require.NotNil(t, city)
}

func TestLookupCityRealReturnsErrDBNotLoaded(t *testing.T) {
	cfg := &MaxMind{}

	city, err := cfg.lookupCityReal("1.1.1.1")
	require.ErrorIs(t, err, ErrDBNotLoaded)
	assert.Nil(t, city)
}

func TestLogLookupCityErrorIncludesSuppressedCount(t *testing.T) {
	oldLimiter := errLogRateLimiter
	oldSuppressed := errLogSuppressedCounts
	oldLogger := log.Logger
	t.Cleanup(func() {
		errLogRateLimiter = oldLimiter
		errLogSuppressedCounts = oldSuppressed
		log.Logger = oldLogger
	})

	errLogRateLimiter = rate.NewLimiter(rate.Every(24*time.Hour), 1)
	errLogSuppressedCounts = xsync.NewMap[string, *atomic.Int64]()

	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)

	err := errors.New("boom")
	logLookupCityError("1.1.1.1", err)
	logLookupCityError("1.1.1.1", err)

	errLogRateLimiter = rate.NewLimiter(rate.Inf, 1)
	logLookupCityError("1.1.1.1", err)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.Len(t, lines, 2)

	var entry map[string]any
	require.NoError(t, json.Unmarshal(lines[1], &entry))
	assert.Equal(t, "failed to lookup city", entry["message"])
	assert.Equal(t, float64(1), entry["suppressed_count"])

	buf.Reset()
	logLookupCityError("1.1.1.1", err)
	entry = map[string]any{}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry))
	_, hasSuppressedCount := entry["suppressed_count"]
	assert.False(t, hasSuppressedCount)
}
