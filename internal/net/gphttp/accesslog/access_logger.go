package accesslog

import (
	"bufio"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/synk"
	"golang.org/x/time/rate"
)

type (
	AccessLogger struct {
		task          *task.Task
		cfg           *Config
		io            AccessLogIO
		buffered      *bufio.Writer
		supportRotate bool

		lineBufPool *synk.BytesPool // buffer pool for formatting a single log line

		errRateLimiter *rate.Limiter

		logger zerolog.Logger

		Formatter
	}

	AccessLogIO interface {
		io.Writer
		sync.Locker
		Name() string // file name or path
	}

	Formatter interface {
		// AppendLog appends a log line to line with or without a trailing newline
		AppendLog(line []byte, req *http.Request, res *http.Response) []byte
	}
)

const MinBufferSize = 4 * kilobyte

const (
	flushInterval  = 30 * time.Second
	rotateInterval = time.Hour
)

func NewAccessLogger(parent task.Parent, cfg *Config) (*AccessLogger, error) {
	var ios []AccessLogIO

	if cfg.Stdout {
		ios = append(ios, stdoutIO)
	}

	if cfg.Path != "" {
		io, err := newFileIO(cfg.Path)
		if err != nil {
			return nil, err
		}
		ios = append(ios, io)
	}

	if len(ios) == 0 {
		return nil, nil
	}

	return NewAccessLoggerWithIO(parent, NewMultiWriter(ios...), cfg), nil
}

func NewMockAccessLogger(parent task.Parent, cfg *Config) *AccessLogger {
	return NewAccessLoggerWithIO(parent, NewMockFile(), cfg)
}

func NewAccessLoggerWithIO(parent task.Parent, io AccessLogIO, cfg *Config) *AccessLogger {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.BufferSize < MinBufferSize {
		cfg.BufferSize = MinBufferSize
	}
	l := &AccessLogger{
		task:           parent.Subtask("accesslog."+io.Name(), true),
		cfg:            cfg,
		io:             io,
		buffered:       bufio.NewWriterSize(io, cfg.BufferSize),
		lineBufPool:    synk.NewBytesPool(1024, synk.DefaultMaxBytes),
		errRateLimiter: rate.NewLimiter(rate.Every(time.Second), 1),
		logger:         logging.With().Str("file", io.Name()).Logger(),
	}

	fmt := CommonFormatter{cfg: &l.cfg.Fields}
	switch l.cfg.Format {
	case FormatCommon:
		l.Formatter = &fmt
	case FormatCombined:
		l.Formatter = &CombinedFormatter{fmt}
	case FormatJSON:
		l.Formatter = &JSONFormatter{fmt}
	default: // should not happen, validation has done by validate tags
		panic("invalid access log format")
	}

	if _, ok := l.io.(supportRotate); ok {
		l.supportRotate = true
	}

	go l.start()
	return l
}

func (l *AccessLogger) Config() *Config {
	return l.cfg
}

func (l *AccessLogger) shouldLog(req *http.Request, res *http.Response) bool {
	if !l.cfg.Filters.StatusCodes.CheckKeep(req, res) ||
		!l.cfg.Filters.Method.CheckKeep(req, res) ||
		!l.cfg.Filters.Headers.CheckKeep(req, res) ||
		!l.cfg.Filters.CIDR.CheckKeep(req, res) {
		return false
	}
	return true
}

func (l *AccessLogger) Log(req *http.Request, res *http.Response) {
	if !l.shouldLog(req, res) {
		return
	}

	line := l.lineBufPool.Get()
	defer l.lineBufPool.Put(line)
	line = l.Formatter.AppendLog(line, req, res)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.lockWrite(line)
}

func (l *AccessLogger) LogError(req *http.Request, err error) {
	l.Log(req, &http.Response{StatusCode: http.StatusInternalServerError, Status: err.Error()})
}

func (l *AccessLogger) ShouldRotate() bool {
	return l.cfg.Retention.IsValid() && l.supportRotate
}

func (l *AccessLogger) Rotate() (result *RotateResult, err error) {
	if !l.ShouldRotate() {
		return nil, nil
	}

	l.io.Lock()
	defer l.io.Unlock()

	return rotateLogFile(l.io.(supportRotate), l.cfg.Retention)
}

func (l *AccessLogger) handleErr(err error) {
	if l.errRateLimiter.Allow() {
		gperr.LogError("failed to write access log", err)
	} else {
		gperr.LogError("too many errors, stopping access log", err)
		l.task.Finish(err)
	}
}

func (l *AccessLogger) start() {
	defer func() {
		defer l.task.Finish(nil)
		defer l.close()
		if err := l.Flush(); err != nil {
			l.handleErr(err)
		}
	}()

	// flushes the buffer every 30 seconds
	flushTicker := time.NewTicker(30 * time.Second)
	defer flushTicker.Stop()

	rotateTicker := time.NewTicker(rotateInterval)
	defer rotateTicker.Stop()

	for {
		select {
		case <-l.task.Context().Done():
			return
		case <-flushTicker.C:
			if err := l.Flush(); err != nil {
				l.handleErr(err)
			}
		case <-rotateTicker.C:
			if !l.ShouldRotate() {
				continue
			}
			l.logger.Info().Msg("rotating access log file")
			if res, err := l.Rotate(); err != nil {
				l.handleErr(err)
			} else if res != nil {
				res.Print(&l.logger)
			} else {
				l.logger.Info().Msg("no rotation needed")
			}
		}
	}
}

func (l *AccessLogger) Flush() error {
	l.io.Lock()
	defer l.io.Unlock()
	return l.buffered.Flush()
}

func (l *AccessLogger) close() {
	if r, ok := l.io.(io.Closer); ok {
		l.io.Lock()
		defer l.io.Unlock()
		r.Close()
	}
}

func (l *AccessLogger) lockWrite(data []byte) {
	l.io.Lock() // prevent concurrent write, i.e. log rotation, other access loggers
	_, err := l.buffered.Write(data)
	l.io.Unlock()
	if err != nil {
		l.handleErr(err)
	} else {
		logging.Trace().Msg("access log flushed to " + l.io.Name())
	}
}
