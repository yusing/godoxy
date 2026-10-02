package maxmind

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/cache"
	"github.com/yusing/goutils/task"
	"golang.org/x/time/rate"
)

func TestLookupCityPropagatesContext(t *testing.T) {
	type contextKey string
	const wantIP = "1.1.1.1"
	const wantValue contextKey = "trace-id"

	instance := &MaxMind{}
	instance.lookupCity = cache.NewKeyFunc(func(ctx context.Context, ipStr string) (*City, error) {
		require.Equal(t, wantIP, ipStr)
		require.Equal(t, "trace-123", ctx.Value(wantValue))
		return &City{}, nil
	}).Build()
	parent := task.GetTestTask(t)
	SetCtx(parent, instance)

	ctx := context.WithValue(parent.Context(), wantValue, "trace-123")
	city, loaded := LookupCity(ctx, &IPInfo{Str: wantIP})
	require.True(t, loaded)
	require.NotNil(t, city)
}

func TestLookupCityRealReturnsErrDBNotLoaded(t *testing.T) {
	cfg := &MaxMind{}

	for _, ip := range []string{"1.1.1.1", "not-an-ip", "fe80::1%eth0"} {
		t.Run(ip, func(t *testing.T) {
			city, err := cfg.lookupCityReal(ip)
			require.ErrorIs(t, err, ErrDBNotLoaded)
			assert.Nil(t, city)
		})
	}
}

func TestLookupCityRealDecodesDatabase(t *testing.T) {
	// Authoritative fixture from maxmind/MaxMind-DB at
	// 276926d23b4109ca5452709bfb5931c338afb34c, test-data/GeoIP2-City-Test.mmdb.
	// Expected records come from source-data/GeoIP2-City-Test.json at that revision.
	// Distributed under the accompanying Apache-2.0 or MIT licenses.
	reader, err := maxminddb.Open("testdata/GeoIP2-City-Test.mmdb")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()) })
	cfg := &MaxMind{}
	cfg.db.Reader = reader

	for _, tt := range []struct {
		name, ip, country, timeZone string
	}{
		{"IPv4", "81.2.69.142", "GB", "Europe/London"},
		{"IPv6", "2001:218::1", "JP", "Asia/Tokyo"},
		{"IPv4-mapped", "::ffff:81.2.69.142", "GB", "Europe/London"},
		{"missing IPv4", "192.0.2.1", "", ""},
		{"missing IPv6", "2001:db8::1", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			city, err := cfg.lookupCityReal(tt.ip)
			require.NoError(t, err)
			require.NotNil(t, city)
			assert.Equal(t, tt.country, city.Country.IsoCode)
			assert.Equal(t, tt.timeZone, city.Location.TimeZone)
		})
	}

	for _, ip := range []string{"", "not-an-ip", "999.1.1.1", "81.2.69.142:80", "[2001:218::1]", "fe80::1%eth0", "2001:218::1%eth0", "::ffff:81.2.69.142%eth0"} {
		t.Run("invalid/"+ip, func(t *testing.T) {
			city, err := cfg.lookupCityReal(ip)
			require.ErrorIs(t, err, ErrInvalidIP)
			assert.Nil(t, city)
		})
	}
}

func TestLookupCityReusesResolvedInfo(t *testing.T) {
	want := &City{}
	want.Country.IsoCode = "BR"
	calls := 0
	instance := &MaxMind{}
	instance.lookupCity = cache.NewKeyFunc(func(context.Context, string) (*City, error) {
		calls++
		return want, nil
	}).Build()
	parent := task.GetTestTask(t)
	SetCtx(parent, instance)
	info := &IPInfo{Str: "200.1.2.3"}

	city, ok := LookupCity(parent.Context(), info)
	require.True(t, ok)
	require.Same(t, want, city)

	// A resolved record is usable without another database or context lookup.
	city, ok = LookupCity(t.Context(), info)
	require.True(t, ok)
	require.Same(t, want, city)
	require.Equal(t, 1, calls)
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
