package v1

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/api/v1/utils"
	"github.com/yusing/go-proxy/internal/common"
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type logEntryRange struct {
	Start, End int
}

type memLogger struct {
	bytes.Buffer
	sync.RWMutex
	notifyLock sync.RWMutex
	connChans  F.Map[chan *logEntryRange, struct{}]

	bufPool sync.Pool // used in hook mode
}

type MemLogger interface {
	io.Writer
	// TODO: hook does not pass in fields, looking for a workaround to do server side log rendering
	zerolog.Hook
}

type buffer struct {
	data []byte
}

const (
	maxMemLogSize         = 16 * 1024
	truncateSize          = maxMemLogSize / 2
	initialWriteChunkSize = 4 * 1024
	hookModeBufSize       = 256
)

var memLoggerInstance = &memLogger{
	connChans: F.NewMapOf[chan *logEntryRange, struct{}](),
	bufPool: sync.Pool{
		New: func() any {
			return &buffer{
				data: make([]byte, 0, hookModeBufSize),
			}
		},
	},
}

func init() {
	if !common.EnableLogStreaming {
		return
	}
	memLoggerInstance.Grow(maxMemLogSize)

	if common.DebugMemLogger {
		ticker := time.NewTicker(1 * time.Second)

		go func() {
			defer ticker.Stop()

			for {
				select {
				case <-task.RootContextCanceled():
					return
				case <-ticker.C:
					logging.Info().Msgf("mem logger size: %d, active conns: %d",
						memLoggerInstance.Len(),
						memLoggerInstance.connChans.Size())
				}
			}
		}()
	}
}

func LogsWS() func(config config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	return memLoggerInstance.ServeHTTP
}

func GetMemLogger() MemLogger {
	return memLoggerInstance
}

func (m *memLogger) truncateIfNeeded(n int) {
	m.RLock()
	needTruncate := m.Len()+n > maxMemLogSize
	m.RUnlock()

	if needTruncate {
		m.Lock()
		defer m.Unlock()
		needTruncate = m.Len()+n > maxMemLogSize
		if !needTruncate {
			return
		}

		m.Truncate(truncateSize)
	}
}

func (m *memLogger) notifyWS(pos, n int) {
	if m.connChans.Size() > 0 {
		timeout := time.NewTimer(1 * time.Second)
		defer timeout.Stop()

		m.notifyLock.RLock()
		defer m.notifyLock.RUnlock()
		m.connChans.Range(func(ch chan *logEntryRange, _ struct{}) bool {
			select {
			case ch <- &logEntryRange{pos, pos + n}:
				return true
			case <-timeout.C:
				logging.Warn().Msg("mem logger: timeout logging to channel")
				return false
			}
		})
		return
	}
}

func (m *memLogger) writeBuf(b []byte) (pos int, err error) {
	m.Lock()
	defer m.Unlock()
	pos = m.Len()
	_, err = m.Buffer.Write(b)
	return
}

// Run implements zerolog.Hook.
func (m *memLogger) Run(e *zerolog.Event, level zerolog.Level, message string) {
	bufStruct := m.bufPool.Get().(*buffer)
	buf := bufStruct.data
	defer func() {
		bufStruct.data = bufStruct.data[:0]
		m.bufPool.Put(bufStruct)
	}()

	buf = logging.FormatLogEntryHTML(level, message, buf)
	n := len(buf)

	m.truncateIfNeeded(n)

	pos, err := m.writeBuf(buf)
	if err != nil {
		// not logging the error here, it will cause Run to be called again = infinite loop
		return
	}

	m.notifyWS(pos, n)
}

// Write implements io.Writer.
func (m *memLogger) Write(p []byte) (n int, err error) {
	n = len(p)
	m.truncateIfNeeded(n)

	pos, err := m.writeBuf(p)
	if err != nil {
		// not logging the error here, it will cause Run to be called again = infinite loop
		return
	}

	m.notifyWS(pos, n)
	return
}

func (m *memLogger) ServeHTTP(config config.ConfigInstance, w http.ResponseWriter, r *http.Request) {
	conn, err := utils.InitiateWS(config, w, r)
	if err != nil {
		utils.HandleErr(w, r, err)
		return
	}

	logCh := make(chan *logEntryRange)
	m.connChans.Store(logCh, struct{}{})

	/* trunk-ignore(golangci-lint/errcheck) */
	defer func() {
		_ = conn.CloseNow()

		m.notifyLock.Lock()
		m.connChans.Delete(logCh)
		close(logCh)
		m.notifyLock.Unlock()
	}()

	if err := m.wsInitial(r.Context(), conn); err != nil {
		utils.HandleErr(w, r, err)
		return
	}

	m.wsStreamLog(r.Context(), conn, logCh)
}

func (m *memLogger) writeBytes(ctx context.Context, conn *websocket.Conn, b []byte) error {
	return conn.Write(ctx, websocket.MessageText, b)
}

func (m *memLogger) wsInitial(ctx context.Context, conn *websocket.Conn) error {
	m.Lock()
	defer m.Unlock()

	return m.writeBytes(ctx, conn, m.Buffer.Bytes())
}

func (m *memLogger) wsStreamLog(ctx context.Context, conn *websocket.Conn, ch <-chan *logEntryRange) {
	for {
		select {
		case <-ctx.Done():
			return
		case logRange := <-ch:
			m.RLock()
			msg := m.Buffer.Bytes()[logRange.Start:logRange.End]
			err := m.writeBytes(ctx, conn, msg)
			m.RUnlock()
			if err != nil {
				return
			}
		}
	}
}
