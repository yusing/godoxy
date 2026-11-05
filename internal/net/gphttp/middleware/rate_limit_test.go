package middleware

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/goutils/task"
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

	serve := func() (int, map[string]any) {
		req := accesslog.WithRequestInfo(httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil))
		rec := httptest.NewRecorder()
		rl.ServeHTTP(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, rec, req)
		cfg := accesslog.DefaultRequestLoggerConfig()
		cfg.Format = accesslog.FormatJSON
		cfg.Path = filepath.Join(t.TempDir(), "access.log")
		logger, err := accesslog.NewAccessLogger(task.GetTestTask(t), cfg)
		expect.NoError(t, err)
		logger.LogRequest(req, rec.Result())
		logger.Flush()
		data, err := os.ReadFile(cfg.Path)
		expect.NoError(t, err)
		var fields map[string]any
		expect.NoError(t, json.Unmarshal(data, &fields))
		return rec.Code, fields
	}
	code, info := serve()
	expect.Equal(t, code, http.StatusOK)
	expect.Equal(t, info["denied_by"], nil)
	code, info = serve()
	expect.Equal(t, code, http.StatusTooManyRequests)
	expect.Equal(t, info["denied_by"], "RateLimit")
}
