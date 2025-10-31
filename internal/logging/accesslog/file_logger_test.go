package accesslog

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yusing/goutils/task"
)

func TestConcurrentFileLoggersShareSameAccessLogIO(t *testing.T) {
	cfg := DefaultRequestLoggerConfig()
	cfg.Path = "test.log"

	loggerCount := runtime.GOMAXPROCS(0)
	accessLogIOs := make([]Writer, loggerCount)

	// make test log file
	file, err := os.Create(cfg.Path)
	assert.NoError(t, err)
	file.Close()
	t.Cleanup(func() {
		assert.NoError(t, os.Remove(cfg.Path))
	})

	var wg sync.WaitGroup
	for i := range loggerCount {
		wg.Go(func() {
			file, err := NewFileIO(cfg.Path)
			assert.NoError(t, err)
			accessLogIOs[i] = file
		})
	}

	wg.Wait()

	firstIO := accessLogIOs[0]
	for _, io := range accessLogIOs {
		assert.Equal(t, firstIO, io)
	}
}

func TestConcurrentAccessLoggerLogAndFlush(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		t.Run(fmt.Sprintf("buffered=%t", buffered), func(t *testing.T) {
			file := NewMockFile(buffered)

			cfg := DefaultRequestLoggerConfig()
			parent := task.RootTask("test", false)

			loggerCount := runtime.GOMAXPROCS(0)
			logCountPerLogger := 10
			loggers := make([]*AccessLogger, loggerCount)

			for i := range loggerCount {
				loggers[i] = NewAccessLoggerWithIO(parent, file, cfg)
			}

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			resp := &http.Response{StatusCode: http.StatusOK}

			var wg sync.WaitGroup
			for _, logger := range loggers {
				wg.Go(func() {
					concurrentLog(logger, req, resp, logCountPerLogger)
				})
			}
			wg.Wait()

			for _, logger := range loggers {
				logger.Close()
			}

			expected := loggerCount * logCountPerLogger
			actual := file.NumLines()
			assert.Equal(t, expected, actual)
		})
	}
}

func concurrentLog(logger *AccessLogger, req *http.Request, resp *http.Response, n int) {
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			logger.Log(req, resp)
			if rand.IntN(2) == 0 {
				logger.Flush()
			}
		})
	}
	wg.Wait()
}
