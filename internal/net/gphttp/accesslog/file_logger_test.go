package accesslog

import (
	"net/http"
	"os"
	"sync"
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"

	"github.com/yusing/go-proxy/internal/task"
)

func TestConcurrentFileLoggersShareSameAccessLogIO(t *testing.T) {
	var wg sync.WaitGroup

	cfg := DefaultConfig()
	cfg.Path = "test.log"
	parent := task.RootTask("test", false)

	loggerCount := 10
	accessLogIOs := make([]AccessLogIO, loggerCount)

	// make test log file
	file, err := os.Create(cfg.Path)
	ExpectNoError(t, err)
	file.Close()
	t.Cleanup(func() {
		ExpectNoError(t, os.Remove(cfg.Path))
	})

	for i := range loggerCount {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			logger, err := NewFileAccessLogger(parent, cfg)
			ExpectNoError(t, err)
			accessLogIOs[index] = logger.io
		}(i)
	}

	wg.Wait()

	firstIO := accessLogIOs[0]
	for _, io := range accessLogIOs {
		ExpectEqual(t, io, firstIO)
	}
}

func TestConcurrentAccessLoggerLogAndFlush(t *testing.T) {
	var file MockFile

	cfg := DefaultConfig()
	cfg.BufferSize = 1024
	parent := task.RootTask("test", false)

	loggerCount := 5
	logCountPerLogger := 10
	loggers := make([]*AccessLogger, loggerCount)

	for i := range loggerCount {
		loggers[i] = NewAccessLogger(parent, &file, cfg)
	}

	var wg sync.WaitGroup
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp := &http.Response{StatusCode: http.StatusOK}

	for _, logger := range loggers {
		wg.Add(1)
		go func(l *AccessLogger) {
			defer wg.Done()
			parallelLog(l, req, resp, logCountPerLogger)
			l.Flush()
		}(logger)
	}

	wg.Wait()

	expected := loggerCount * logCountPerLogger
	actual := file.LineCount()
	ExpectEqual(t, actual, expected)
}

func parallelLog(logger *AccessLogger, req *http.Request, resp *http.Response, n int) {
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			logger.Log(req, resp)
		}()
	}
	wg.Wait()
}
