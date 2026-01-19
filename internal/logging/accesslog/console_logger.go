package accesslog

import (
	"net/http"
	"os"

	"github.com/rs/zerolog"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
)

type ConsoleLogger struct {
	cfg *Config

	formatter ConsoleFormatter
}

var stdoutLogger = func() *zerolog.Logger {
	l := zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = os.Stdout
		w.TimeFormat = zerolog.TimeFieldFormat
		w.FieldsOrder = []string{
			"uri", "protocol", "type", "size",
			"useragent", "query", "headers", "cookies",
			"error", "iso_code", "time_zone"}
	})).With().Str("level", zerolog.InfoLevel.String()).Timestamp().Logger()
	return &l
}()

// placeholder for console logger
var stdout File = &sharedFileHandle{}

func NewConsoleLogger(cfg *Config) AccessLogger {
	if cfg == nil {
		panic("accesslog: NewConsoleLogger called with nil config")
	}
	l := &ConsoleLogger{
		cfg: cfg,
	}
	if cfg.req != nil {
		l.formatter = ConsoleFormatter{cfg: &cfg.req.Fields}
	}
	return l
}

func (l *ConsoleLogger) Config() *Config {
	return l.cfg
}

func (l *ConsoleLogger) LogRequest(req *http.Request, res *http.Response) {
	if !l.cfg.ShouldLogRequest(req, res) {
		return
	}

	l.formatter.LogRequestZeroLog(stdoutLogger, req, res)
}

func (l *ConsoleLogger) LogError(req *http.Request, err error) {
	log := stdoutLogger.With().Err(err).Logger()
	l.formatter.LogRequestZeroLog(&log, req, internalErrorResponse)
}

func (l *ConsoleLogger) LogACL(info *maxmind.IPInfo, blocked bool) {
	ConsoleACLFormatter{}.LogACLZeroLog(stdoutLogger, info, blocked)
}

func (l *ConsoleLogger) Flush() {
	// No-op for console logger
}

func (l *ConsoleLogger) Close() error {
	// No-op for console logger
	return nil
}
