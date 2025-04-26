package accesslog

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"
	acl "github.com/yusing/go-proxy/internal/acl/types"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/synk"
	"golang.org/x/time/rate"
)

type (
	AccessLogger struct {
		task *task.Task
		cfg  *Config

		closer        []io.Closer
		supportRotate []supportRotate
		writer        *bufio.Writer
		writeLock     sync.Mutex
		closed        bool

		lineBufPool *synk.BytesPool // buffer pool for formatting a single log line

		errRateLimiter *rate.Limiter

		logger zerolog.Logger

		RequestFormatter
		ACLFormatter
	}

	WriterWithName interface {
		io.Writer
		Name() string // file name or path
	}

	RequestFormatter interface {
		// AppendRequestLog appends a log line to line with or without a trailing newline
		AppendRequestLog(line []byte, req *http.Request, res *http.Response) []byte
	}
	ACLFormatter interface {
		// AppendACLLog appends a log line to line with or without a trailing newline
		AppendACLLog(line []byte, info *acl.IPInfo, blocked bool) []byte
	}
)

const (
	StdoutbufSize = 64
	MinBufferSize = 4 * kilobyte
	MaxBufferSize = 1 * megabyte
)

const (
	flushInterval  = 30 * time.Second
	rotateInterval = time.Hour
)

const (
	errRateLimit = 200 * time.Millisecond
	errBurst     = 5
)

func NewAccessLogger(parent task.Parent, cfg AnyConfig) (*AccessLogger, error) {
	io, err := cfg.IO()
	if err != nil {
		return nil, err
	}
	return NewAccessLoggerWithIO(parent, io, cfg), nil
}

func NewMockAccessLogger(parent task.Parent, cfg *RequestLoggerConfig) *AccessLogger {
	return NewAccessLoggerWithIO(parent, NewMockFile(), cfg)
}

func unwrap[Writer any](w io.Writer) []Writer {
	var result []Writer
	if unwrapped, ok := w.(MultiWriterInterface); ok {
		for _, w := range unwrapped.Unwrap() {
			if unwrapped, ok := w.(Writer); ok {
				result = append(result, unwrapped)
			}
		}
		return result
	}
	if unwrapped, ok := w.(Writer); ok {
		return []Writer{unwrapped}
	}
	return nil
}

func NewAccessLoggerWithIO(parent task.Parent, writer WriterWithName, anyCfg AnyConfig) *AccessLogger {
	cfg := anyCfg.ToConfig()
	if cfg.BufferSize == 0 {
		cfg.BufferSize = DefaultBufferSize
	}
	if cfg.BufferSize < MinBufferSize {
		cfg.BufferSize = MinBufferSize
	}
	if cfg.BufferSize > MaxBufferSize {
		cfg.BufferSize = MaxBufferSize
	}
	if _, ok := writer.(*os.File); ok {
		cfg.BufferSize = StdoutbufSize
	}

	l := &AccessLogger{
		task:           parent.Subtask("accesslog."+writer.Name(), true),
		cfg:            cfg,
		writer:         bufio.NewWriterSize(writer, cfg.BufferSize),
		lineBufPool:    synk.NewBytesPool(512, 8192),
		errRateLimiter: rate.NewLimiter(rate.Every(errRateLimit), errBurst),
		logger:         logging.With().Str("file", writer.Name()).Logger(),
	}

	l.supportRotate = unwrap[supportRotate](writer)
	l.closer = unwrap[io.Closer](writer)

	if cfg.req != nil {
		fmt := CommonFormatter{cfg: &cfg.req.Fields}
		switch cfg.req.Format {
		case FormatCommon:
			l.RequestFormatter = &fmt
		case FormatCombined:
			l.RequestFormatter = &CombinedFormatter{fmt}
		case FormatJSON:
			l.RequestFormatter = &JSONFormatter{fmt}
		default: // should not happen, validation has done by validate tags
			panic("invalid access log format")
		}
	} else {
		l.ACLFormatter = ACLLogFormatter{}
	}

	go l.start()
	return l
}

func (l *AccessLogger) Config() *Config {
	return l.cfg
}

func (l *AccessLogger) shouldLog(req *http.Request, res *http.Response) bool {
	if !l.cfg.req.Filters.StatusCodes.CheckKeep(req, res) ||
		!l.cfg.req.Filters.Method.CheckKeep(req, res) ||
		!l.cfg.req.Filters.Headers.CheckKeep(req, res) ||
		!l.cfg.req.Filters.CIDR.CheckKeep(req, res) {
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
	line = l.AppendRequestLog(line, req, res)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.write(line)
}

func (l *AccessLogger) LogError(req *http.Request, err error) {
	l.Log(req, &http.Response{StatusCode: http.StatusInternalServerError, Status: err.Error()})
}

func (l *AccessLogger) LogACL(info *acl.IPInfo, blocked bool) {
	line := l.lineBufPool.Get()
	defer l.lineBufPool.Put(line)
	line = l.ACLFormatter.AppendACLLog(line, info, blocked)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.write(line)
}

func (l *AccessLogger) ShouldRotate() bool {
	return l.supportRotate != nil && l.cfg.Retention.IsValid()
}

func (l *AccessLogger) Rotate() (result *RotateResult, err error) {
	if !l.ShouldRotate() {
		return nil, nil
	}

	l.writer.Flush()
	l.writeLock.Lock()
	defer l.writeLock.Unlock()

	result = new(RotateResult)
	for _, sr := range l.supportRotate {
		r, err := rotateLogFile(sr, l.cfg.Retention)
		if err != nil {
			return nil, err
		}
		if r != nil {
			result.Add(r)
		}
	}
	return result, nil
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
		l.Flush()
		l.Close()
		l.task.Finish(nil)
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
			l.Flush()
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

func (l *AccessLogger) Close() error {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return nil
	}
	if l.closer != nil {
		for _, c := range l.closer {
			c.Close()
		}
	}
	l.closed = true
	return nil
}

func (l *AccessLogger) Flush() {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	if err := l.writer.Flush(); err != nil {
		l.handleErr(err)
	}
}

func (l *AccessLogger) write(data []byte) {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	_, err := l.writer.Write(data)
	if err != nil {
		l.handleErr(err)
	}
}
