package accesslog

import (
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	gperr "github.com/yusing/goutils/errs"
	ioutils "github.com/yusing/goutils/io"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
	"golang.org/x/time/rate"
)

type (
	AccessLogger interface {
		Log(req *http.Request, res *http.Response)
		LogError(req *http.Request, err error)
		LogACL(info *maxmind.IPInfo, blocked bool)

		Config() *Config

		Flush()
		Close() error
	}

	accessLogger struct {
		task *task.Task
		cfg  *Config

		writer        BufferedWriter
		supportRotate SupportRotate
		writeLock     *sync.Mutex
		closed        bool

		writeCount int64
		bufSize    int

		errRateLimiter *rate.Limiter

		logger zerolog.Logger

		RequestFormatter
		ACLFormatter
	}

	Writer interface {
		io.WriteCloser
		ShouldBeBuffered() bool
		Name() string // file name or path
	}

	SupportRotate interface {
		io.Writer
		supportRotate
		Name() string
	}

	AccessLogRotater interface {
		Rotate(result *RotateResult) (rotated bool, err error)
	}

	RequestFormatter interface {
		// AppendRequestLog appends a log line to line with or without a trailing newline
		AppendRequestLog(line []byte, req *http.Request, res *http.Response) []byte
	}
	ACLFormatter interface {
		// AppendACLLog appends a log line to line with or without a trailing newline
		AppendACLLog(line []byte, info *maxmind.IPInfo, blocked bool) []byte
	}
)

var writerLocks = xsync.NewMap[string, *sync.Mutex]()

const (
	InitialBufferSize = 4 * kilobyte
	MaxBufferSize     = 8 * megabyte

	bufferAdjustInterval = 5 * time.Second // How often we check & adjust
)

const defaultRotateInterval = time.Hour

const (
	errRateLimit = 200 * time.Millisecond
	errBurst     = 5
)

var bytesPool = synk.GetUnsizedBytesPool()

func NewAccessLogger(parent task.Parent, cfg AnyConfig) (AccessLogger, error) {
	writers, err := cfg.Writers()
	if err != nil {
		return nil, err
	}

	return NewMultiAccessLogger(parent, cfg, writers), nil
}

func NewMockAccessLogger(parent task.Parent, cfg *RequestLoggerConfig) AccessLogger {
	return NewAccessLoggerWithIO(parent, NewMockFile(true), cfg)
}

func NewAccessLoggerWithIO(parent task.Parent, writer Writer, anyCfg AnyConfig) AccessLogger {
	cfg := anyCfg.ToConfig()
	if cfg.RotateInterval == 0 {
		cfg.RotateInterval = defaultRotateInterval
	}

	l := &accessLogger{
		task:           parent.Subtask("accesslog."+writer.Name(), true),
		cfg:            cfg,
		bufSize:        InitialBufferSize,
		errRateLimiter: rate.NewLimiter(rate.Every(errRateLimit), errBurst),
		logger:         log.With().Str("file", writer.Name()).Logger(),
	}

	l.writeLock, _ = writerLocks.LoadOrStore(writer.Name(), &sync.Mutex{})

	if writer.ShouldBeBuffered() {
		l.writer = ioutils.NewBufferedWriter(writer, InitialBufferSize)
	} else {
		l.writer = NewUnbufferedWriter(writer)
	}

	if supportRotate, ok := writer.(SupportRotate); ok {
		l.supportRotate = supportRotate
	}

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

func (l *accessLogger) Config() *Config {
	return l.cfg
}

func (l *accessLogger) shouldLog(req *http.Request, res *http.Response) bool {
	if !l.cfg.req.Filters.StatusCodes.CheckKeep(req, res) ||
		!l.cfg.req.Filters.Method.CheckKeep(req, res) ||
		!l.cfg.req.Filters.Headers.CheckKeep(req, res) ||
		!l.cfg.req.Filters.CIDR.CheckKeep(req, res) {
		return false
	}
	return true
}

func (l *accessLogger) Log(req *http.Request, res *http.Response) {
	if !l.shouldLog(req, res) {
		return
	}

	line := bytesPool.Get()
	line = l.AppendRequestLog(line, req, res)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.write(line)
	bytesPool.Put(line)
}

func (l *accessLogger) LogError(req *http.Request, err error) {
	l.Log(req, &http.Response{StatusCode: http.StatusInternalServerError, Status: err.Error()})
}

func (l *accessLogger) LogACL(info *maxmind.IPInfo, blocked bool) {
	line := bytesPool.Get()
	line = l.AppendACLLog(line, info, blocked)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.write(line)
	bytesPool.Put(line)
}

func (l *accessLogger) ShouldRotate() bool {
	return l.supportRotate != nil && l.cfg.Retention.IsValid()
}

func (l *accessLogger) Rotate(result *RotateResult) (rotated bool, err error) {
	if !l.ShouldRotate() {
		return false, nil
	}

	l.Flush()
	l.writeLock.Lock()
	defer l.writeLock.Unlock()

	rotated, err = rotateLogFile(l.supportRotate, l.cfg.Retention, result)
	return
}

func (l *accessLogger) handleErr(err error) {
	if l.errRateLimiter.Allow() {
		gperr.LogError("failed to write access log", err, &l.logger)
	} else {
		gperr.LogError("too many errors, stopping access log", err, &l.logger)
		l.task.Finish(err)
	}
}

func (l *accessLogger) start() {
	defer func() {
		l.Flush()
		l.Close()
		l.task.Finish(nil)
	}()

	rotateTicker := time.NewTicker(l.cfg.RotateInterval)
	defer rotateTicker.Stop()

	bufAdjTicker := time.NewTicker(bufferAdjustInterval)
	defer bufAdjTicker.Stop()

	for {
		select {
		case <-l.task.Context().Done():
			return
		case <-rotateTicker.C:
			if !l.ShouldRotate() {
				continue
			}
			l.logger.Info().Msg("rotating access log file")
			var res RotateResult
			if rotated, err := l.Rotate(&res); err != nil {
				l.handleErr(err)
			} else if rotated {
				res.Print(&l.logger)
			} else {
				l.logger.Info().Msg("no rotation needed")
			}
		case <-bufAdjTicker.C:
			l.adjustBuffer()
		}
	}
}

func (l *accessLogger) Close() error {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return nil
	}
	l.writer.Flush()
	l.closed = true
	return l.writer.Close()
}

func (l *accessLogger) Flush() {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	l.writer.Flush()
}

func (l *accessLogger) write(data []byte) {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	n, err := l.writer.Write(data)
	if err != nil {
		l.handleErr(err)
	} else if n < len(data) {
		l.handleErr(gperr.Errorf("%w, writing %d bytes, only %d written", io.ErrShortWrite, len(data), n))
	}
	atomic.AddInt64(&l.writeCount, int64(n))
}

func (l *accessLogger) adjustBuffer() {
	wps := int(atomic.SwapInt64(&l.writeCount, 0)) / int(bufferAdjustInterval.Seconds())
	origBufSize := l.bufSize
	newBufSize := origBufSize

	halfDiff := (wps - origBufSize) / 2
	if halfDiff < 0 {
		halfDiff = -halfDiff
	}
	step := max(halfDiff, wps/2)

	switch {
	case origBufSize < wps:
		newBufSize += step
		if newBufSize > MaxBufferSize {
			newBufSize = MaxBufferSize
		}
	case origBufSize > wps:
		newBufSize -= step
		if newBufSize < InitialBufferSize {
			newBufSize = InitialBufferSize
		}
	}

	if newBufSize == origBufSize {
		return
	}

	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}

	l.logger.Debug().
		Str("wps", strutils.FormatByteSize(wps)).
		Str("old", strutils.FormatByteSize(origBufSize)).
		Str("new", strutils.FormatByteSize(newBufSize)).
		Msg("adjusted buffer size")

	err := l.writer.Resize(newBufSize)
	if err != nil {
		l.handleErr(err)
		return
	}
	l.bufSize = newBufSize
}
