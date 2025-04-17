package accesslog

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/synk"
)

type (
	AccessLogger struct {
		task     *task.Task
		cfg      *Config
		io       AccessLogIO
		buffered *bufio.Writer

		lineBufPool *synk.BytesPool // buffer pool for formatting a single log line
		Formatter
	}

	AccessLogIO interface {
		io.Writer
		sync.Locker
		Name() string // file name or path
	}

	supportRotate interface {
		io.ReadWriteCloser
		io.ReadWriteSeeker
		io.ReaderAt
		Truncate(size int64) error
	}

	Formatter interface {
		// Format writes a log line to line without a trailing newline
		Format(line *bytes.Buffer, req *http.Request, res *http.Response)
		SetGetTimeNow(getTimeNow func() time.Time)
	}
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
	return NewAccessLoggerWithIO(parent, &MockFile{}, cfg)
}

func NewAccessLoggerWithIO(parent task.Parent, io AccessLogIO, cfg *Config) *AccessLogger {
	if cfg.BufferSize == 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.BufferSize < 4096 {
		cfg.BufferSize = 4096
	}
	l := &AccessLogger{
		task:     parent.Subtask("accesslog."+io.Name(), true),
		cfg:      cfg,
		io:       io,
		buffered: bufio.NewWriterSize(io, cfg.BufferSize),
	}

	fmt := CommonFormatter{cfg: &l.cfg.Fields, GetTimeNow: time.Now}
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

	l.lineBufPool = synk.NewBytesPool(1024, synk.DefaultMaxBytes)
	go l.start()
	return l
}

func (l *AccessLogger) checkKeep(req *http.Request, res *http.Response) bool {
	if !l.cfg.Filters.StatusCodes.CheckKeep(req, res) ||
		!l.cfg.Filters.Method.CheckKeep(req, res) ||
		!l.cfg.Filters.Headers.CheckKeep(req, res) ||
		!l.cfg.Filters.CIDR.CheckKeep(req, res) {
		return false
	}
	return true
}

func (l *AccessLogger) Log(req *http.Request, res *http.Response) {
	if !l.checkKeep(req, res) {
		return
	}

	line := l.lineBufPool.Get()
	defer l.lineBufPool.Put(line)
	l.Formatter.Format(bytes.NewBuffer(line), req, res)
	line = append(line, '\n')
	l.write(line)
}

func (l *AccessLogger) LogError(req *http.Request, err error) {
	l.Log(req, &http.Response{StatusCode: http.StatusInternalServerError, Status: err.Error()})
}

func (l *AccessLogger) Config() *Config {
	return l.cfg
}

func (l *AccessLogger) Rotate() error {
	if l.cfg.Retention == nil {
		return nil
	}
	l.io.Lock()
	defer l.io.Unlock()

	return l.rotate()
}

func (l *AccessLogger) handleErr(err error) {
	gperr.LogError("failed to write access log", err)
}

func (l *AccessLogger) start() {
	defer func() {
		if err := l.Flush(); err != nil {
			l.handleErr(err)
		}
		l.close()
		l.task.Finish(nil)
	}()

	// flushes the buffer every 30 seconds
	flushTicker := time.NewTicker(30 * time.Second)
	defer flushTicker.Stop()

	for {
		select {
		case <-l.task.Context().Done():
			return
		case <-flushTicker.C:
			if err := l.Flush(); err != nil {
				l.handleErr(err)
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
	l.io.Lock()
	defer l.io.Unlock()
	if r, ok := l.io.(io.Closer); ok {
		r.Close()
	}
}

func (l *AccessLogger) write(data []byte) {
	l.io.Lock() // prevent concurrent write, i.e. log rotation, other access loggers
	_, err := l.buffered.Write(data)
	l.io.Unlock()
	if err != nil {
		l.handleErr(err)
	} else {
		logging.Trace().Msg("access log flushed to " + l.io.Name())
	}
}
