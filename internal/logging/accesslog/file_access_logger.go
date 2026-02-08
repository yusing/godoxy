package accesslog

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	ioutils "github.com/yusing/goutils/io"
	strutils "github.com/yusing/goutils/strings"
	"github.com/yusing/goutils/synk"
	"github.com/yusing/goutils/task"
	"golang.org/x/time/rate"
)

type (
	File interface {
		io.WriteCloser
		supportRotate
		Name() string
	}

	fileAccessLogger struct {
		task *task.Task
		cfg  *Config

		writer    BufferedWriter
		file      File
		writeLock *sync.Mutex
		closed    bool

		writeCount int64
		bufSize    int

		errRateLimiter *rate.Limiter

		logger zerolog.Logger

		RequestFormatter
		ACLLogFormatter
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
var sizedPool = synk.GetSizedBytesPool()

func NewFileAccessLogger(parent task.Parent, file File, anyCfg AnyConfig) AccessLogger {
	cfg := anyCfg.ToConfig()
	if cfg.RotateInterval == 0 {
		cfg.RotateInterval = defaultRotateInterval
	}

	name := file.Name()
	l := &fileAccessLogger{
		task:           parent.Subtask("accesslog."+name, true),
		cfg:            cfg,
		bufSize:        InitialBufferSize,
		errRateLimiter: rate.NewLimiter(rate.Every(errRateLimit), errBurst),
		logger:         log.With().Str("file", name).Logger(),
	}

	l.writeLock, _ = writerLocks.LoadOrStore(name, &sync.Mutex{})

	l.writer = ioutils.NewBufferedWriter(file, InitialBufferSize)
	l.file = file

	if cfg.req != nil {
		switch cfg.req.Format {
		case FormatCommon:
			l.RequestFormatter = CommonFormatter{cfg: &cfg.req.Fields}
		case FormatCombined:
			l.RequestFormatter = CombinedFormatter{CommonFormatter{cfg: &cfg.req.Fields}}
		case FormatJSON:
			l.RequestFormatter = JSONFormatter{cfg: &cfg.req.Fields}
		default: // should not happen, validation has done by validate tags
			panic("invalid access log format")
		}
	}

	go l.start()
	return l
}

func (l *fileAccessLogger) Config() *Config {
	return l.cfg
}

func (l *fileAccessLogger) LogRequest(req *http.Request, res *http.Response) {
	if !l.cfg.ShouldLogRequest(req, res) {
		return
	}

	line := bytesPool.GetBuffer()
	defer bytesPool.PutBuffer(line)
	l.AppendRequestLog(line, req, res)
	// line is never empty
	if line.Bytes()[line.Len()-1] != '\n' {
		line.WriteByte('\n')
	}
	l.write(line.Bytes())
}

var internalErrorResponse = &http.Response{
	StatusCode: http.StatusInternalServerError,
	Status:     http.StatusText(http.StatusInternalServerError),
}

func (l *fileAccessLogger) LogError(req *http.Request, err error) {
	l.LogRequest(req, internalErrorResponse)
}

func (l *fileAccessLogger) LogACL(info *maxmind.IPInfo, blocked bool) {
	line := bytesPool.GetBuffer()
	defer bytesPool.PutBuffer(line)
	l.AppendACLLog(line, info, blocked)
	// line is never empty
	if line.Bytes()[line.Len()-1] != '\n' {
		line.WriteByte('\n')
	}
	l.write(line.Bytes())
}

func (l *fileAccessLogger) ShouldRotate() bool {
	return l.cfg.Retention.IsValid()
}

func (l *fileAccessLogger) Rotate(result *RotateResult) (rotated bool, err error) {
	if !l.ShouldRotate() {
		return false, nil
	}

	l.Flush()
	l.writeLock.Lock()
	defer l.writeLock.Unlock()

	rotated, err = rotateLogFile(l.file, l.cfg.Retention, result)
	return
}

func (l *fileAccessLogger) handleErr(err error) {
	if l.errRateLimiter.Allow() {
		l.logger.Err(err).Msg("failed to write access log")
	} else {
		l.logger.Err(err).Msg("too many errors, stopping access log")
		l.task.Finish(err)
	}
}

func (l *fileAccessLogger) start() {
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

func (l *fileAccessLogger) Close() error {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return nil
	}
	l.writer.Flush()
	l.closed = true
	return l.writer.Close()
}

func (l *fileAccessLogger) Flush() {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	l.writer.Flush()
}

func (l *fileAccessLogger) write(data []byte) {
	l.writeLock.Lock()
	defer l.writeLock.Unlock()
	if l.closed {
		return
	}
	n, err := l.writer.Write(data)
	if err != nil {
		l.handleErr(err)
	} else if n < len(data) {
		l.handleErr(fmt.Errorf("%w, writing %d bytes, only %d written", io.ErrShortWrite, len(data), n))
	}
	atomic.AddInt64(&l.writeCount, int64(n))
}

func (l *fileAccessLogger) adjustBuffer() {
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
