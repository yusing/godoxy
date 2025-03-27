package middleware

import (
	"net/http"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestRateLimit(t *testing.T) {
	opts := OptionsRaw{
		"average": "10",
		"burst":   "10",
		"period":  "1s",
	}

	rl, err := RateLimiter.New(opts)
	ExpectNoError(t, err)
	for range 10 {
		result, err := newMiddlewareTest(rl, nil)
		ExpectNoError(t, err)
		ExpectEqual(t, result.ResponseStatus, http.StatusOK)
	}
	result, err := newMiddlewareTest(rl, nil)
	ExpectNoError(t, err)
	ExpectEqual(t, result.ResponseStatus, http.StatusTooManyRequests)
}
