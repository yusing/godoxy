package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yusing/godoxy/internal/logging/accesslog"
	expect "github.com/yusing/goutils/testing"
)

func TestRateLimit(t *testing.T) {
	opts := OptionsRaw{
		"average": "10",
		"burst":   "10",
		"period":  "1s",
	}

	rl, err := RateLimiter.New(opts)
	expect.NoError(t, err)
	for range 10 {
		result, err := newMiddlewareTest(rl, nil)
		expect.NoError(t, err)
		expect.Equal(t, result.ResponseStatus, http.StatusOK)
	}
	result, err := newMiddlewareTest(rl, nil)
	expect.NoError(t, err)
	expect.Equal(t, result.ResponseStatus, http.StatusTooManyRequests)
}

func TestRateLimitRecordsDenial(t *testing.T) {
	rl, err := RateLimiter.New(OptionsRaw{"average": "1", "burst": "1", "period": "1m"})
	expect.NoError(t, err)

	serve := func() (int, accesslog.RequestInfo) {
		req := accesslog.WithRequestInfo(httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil))
		rec := httptest.NewRecorder()
		rl.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, rec, req)
		info, _ := accesslog.RequestInfoOf(req)
		return rec.Code, info
	}
	code, info := serve()
	expect.Equal(t, code, http.StatusOK)
	expect.Equal(t, info.DeniedBy, "")
	code, info = serve()
	expect.Equal(t, code, http.StatusTooManyRequests)
	expect.Equal(t, info.DeniedBy, "RateLimit")
}
