package accesslog

import (
	"bytes"
	"net/http"

	"github.com/rs/zerolog"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/goutils/task"
)

type (
	AccessLogger interface {
		LogRequest(req *http.Request, res *http.Response)
		LogError(req *http.Request, err error)
		LogACL(info *maxmind.IPInfo, blocked bool)

		Config() *Config

		Flush()
		Close() error
	}

	AccessLogRotater interface {
		Rotate(result *RotateResult) (rotated bool, err error)
	}

	RequestFormatter interface {
		// AppendRequestLog appends a log line to line with or without a trailing newline
		AppendRequestLog(line *bytes.Buffer, req *http.Request, res *http.Response)
	}
	RequestFormatterZeroLog interface {
		// LogRequestZeroLog logs a request log to the logger
		LogRequestZeroLog(logger *zerolog.Logger, req *http.Request, res *http.Response)
	}
	ACLFormatter interface {
		// AppendACLLog appends a log line to line with or without a trailing newline
		AppendACLLog(line *bytes.Buffer, info *maxmind.IPInfo, blocked bool)
		// LogACLZeroLog logs an ACL log to the logger
		LogACLZeroLog(logger *zerolog.Logger, info *maxmind.IPInfo, blocked bool)
	}
)

func NewAccessLogger(parent task.Parent, cfg AnyConfig) (AccessLogger, error) {
	writers, err := cfg.Writers()
	if err != nil {
		return nil, err
	}

	return NewMultiAccessLogger(parent, cfg, writers), nil
}

func NewMockAccessLogger(parent task.Parent, cfg *RequestLoggerConfig) AccessLogger {
	return NewFileAccessLogger(parent, NewMockFile(true), cfg)
}
