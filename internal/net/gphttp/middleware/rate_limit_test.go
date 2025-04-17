package middleware

import (
	"net/http"
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
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
