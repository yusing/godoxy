package accesslog

import (
	"net/http"

	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
)

type MultiAccessLogger struct {
	accessLoggers []AccessLogger
}

// NewMultiAccessLogger creates a new AccessLogger that writes to multiple writers.
//
// If there is only one writer, it will return a single AccessLogger.
// Otherwise, it will return a MultiAccessLogger that writes to all the writers.
func NewMultiAccessLogger(parent task.Parent, cfg AnyConfig, writers []Writer) AccessLogger {
	if len(writers) == 1 {
		return NewAccessLoggerWithIO(parent, writers[0], cfg)
	}

	accessLoggers := make([]AccessLogger, len(writers))
	for i, writer := range writers {
		accessLoggers[i] = NewAccessLoggerWithIO(parent, writer, cfg)
	}
	return &MultiAccessLogger{accessLoggers}
}

func (m *MultiAccessLogger) Config() *Config {
	return m.accessLoggers[0].Config()
}

func (m *MultiAccessLogger) Log(req *http.Request, res *http.Response) {
	for _, accessLogger := range m.accessLoggers {
		accessLogger.Log(req, res)
	}
}

func (m *MultiAccessLogger) LogError(req *http.Request, err error) {
	for _, accessLogger := range m.accessLoggers {
		accessLogger.LogError(req, err)
	}
}

func (m *MultiAccessLogger) LogACL(info *maxmind.IPInfo, blocked bool) {
	for _, accessLogger := range m.accessLoggers {
		accessLogger.LogACL(info, blocked)
	}
}

func (m *MultiAccessLogger) Flush() {
	for _, accessLogger := range m.accessLoggers {
		accessLogger.Flush()
	}
}

func (m *MultiAccessLogger) Close() error {
	for _, accessLogger := range m.accessLoggers {
		accessLogger.Close()
	}
	return nil
}
