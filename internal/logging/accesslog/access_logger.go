package accesslog

import (
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/gperr"
	maxmind "github.com/yusing/go-proxy/internal/maxmind/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
	"github.com/yusing/go-proxy/internal/utils/synk"
	"golang.org/x/time/rate"
)

type (
	AccessLogger struct {
		task *task.Task
		cfg  *Config

		rawWriter     io.Writer
		closer        []io.Closer
		supportRotate []supportRotate
		writer        *utils.BufferedWriter
		writeLock     sync.Mutex
		closed        bool

		writeCount int64
		bufSize    int

		errRateLimiter *rate.Limiter

		logger zerolog.Logger

		RequestFormatter
		ACLFormatter
	}

	WriterWithName interface {
		io.Writer
		Name() string // file name or path
	}

	SupportRotate interface {
		io.Writer
		supportRotate
		Name() string
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

const (
	MinBufferSize = 4 * kilobyte
	MaxBufferSize = 8 * megabyte

	bufferAdjustInterval = 5 * time.Second // How often we check & adjust
)

const defaultRotateInterval = time.Hour

const (
	errRateLimit = 200 * time.Millisecond
	errBurst     = 5
)

var lineBufPool = synk.GetBytesPoolWithUniqueMemory()

func NewAccessLogger(parent task.Parent, cfg AnyConfig) (*AccessLogger, error) {
	io, err := cfg.IO()
	if err != nil {
		return nil, err
	}
	if io == nil {
		return nil, nil //nolint:nilnil
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
	if cfg.RotateInterval == 0 {
		cfg.RotateInterval = defaultRotateInterval
	}

	l := &AccessLogger{
		task:           parent.Subtask("accesslog."+writer.Name(), true),
		cfg:            cfg,
		rawWriter:      writer,
		writer:         utils.NewBufferedWriter(writer, MinBufferSize),
		bufSize:        MinBufferSize,
		errRateLimiter: rate.NewLimiter(rate.Every(errRateLimit), errBurst),
		logger:         log.With().Str("file", writer.Name()).Logger(),
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

	line := lineBufPool.Get()
	defer lineBufPool.Put(line)
	line = l.AppendRequestLog(line, req, res)
	if line[len(line)-1] != '\n' {
		line = append(line, '\n')
	}
	l.write(line)
}

func (l *AccessLogger) LogError(req *http.Request, err error) {
	l.Log(req, &http.Response{StatusCode: http.StatusInternalServerError, Status: err.Error()})
}

func (l *AccessLogger) LogACL(info *maxmind.IPInfo, blocked bool) {
	line := lineBufPool.Get()
	defer lineBufPool.Put(line)
	line = l.AppendACLLog(line, info, blocked)
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
		return nil, nil //nolint:nilnil
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
		gperr.LogError("failed to write access log", err, &l.logger)
	} else {
		gperr.LogError("too many errors, stopping access log", err, &l.logger)
		l.task.Finish(err)
	}
}

func (l *AccessLogger) start() {
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
			if res, err := l.Rotate(); err != nil {
				l.handleErr(err)
			} else if res != nil {
				res.Print(&l.logger)
			} else {
				l.logger.Info().Msg("no rotation needed")
			}
		case <-bufAdjTicker.C:
			l.adjustBuffer()
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
	l.writer.Release()
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
	n, err := l.writer.Write(data)
	if err != nil {
		l.handleErr(err)
	} else if n < len(data) {
		l.handleErr(gperr.Errorf("%w, writing %d bytes, only %d written", io.ErrShortWrite, len(data), n))
	}
	atomic.AddInt64(&l.writeCount, int64(n))
}

func (l *AccessLogger) adjustBuffer() {
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
		if newBufSize < MinBufferSize {
			newBufSize = MinBufferSize
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
