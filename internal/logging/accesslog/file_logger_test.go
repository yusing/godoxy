package accesslog

import (
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func TestConcurrentFileLoggersShareSameAccessLogIO(t *testing.T) {
	var wg sync.WaitGroup

	cfg := DefaultRequestLoggerConfig()
	cfg.Path = "test.log"

	loggerCount := 10
	accessLogIOs := make([]WriterWithName, loggerCount)

	// make test log file
	file, err := os.Create(cfg.Path)
	expect.NoError(t, err)
	file.Close()
	t.Cleanup(func() {
		expect.NoError(t, os.Remove(cfg.Path))
	})

	for i := range loggerCount {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			file, err := NewFileIO(cfg.Path)
			expect.NoError(t, err)
			accessLogIOs[index] = file
		}(i)
	}

	wg.Wait()

	firstIO := accessLogIOs[0]
	for _, io := range accessLogIOs {
		expect.Equal(t, io, firstIO)
	}
}

func TestConcurrentAccessLoggerLogAndFlush(t *testing.T) {
	file := NewMockFile()

	cfg := DefaultRequestLoggerConfig()
	parent := task.RootTask("test", false)

	loggerCount := 5
	logCountPerLogger := 10
	loggers := make([]*AccessLogger, loggerCount)

	for i := range loggerCount {
		loggers[i] = NewAccessLoggerWithIO(parent, file, cfg)
	}

	var wg sync.WaitGroup
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp := &http.Response{StatusCode: http.StatusOK}

	wg.Add(len(loggers))
	for _, logger := range loggers {
		go func(l *AccessLogger) {
			defer wg.Done()
			parallelLog(l, req, resp, logCountPerLogger)
			l.Flush()
		}(logger)
	}

	wg.Wait()

	expected := loggerCount * logCountPerLogger
	actual := file.NumLines()
	expect.Equal(t, actual, expected)
}

func parallelLog(logger *AccessLogger, req *http.Request, resp *http.Response, n int) {
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			logger.Log(req, resp)
		})
	}
	wg.Wait()
}
