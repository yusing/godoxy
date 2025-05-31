package memlogger

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/go-proxy/internal/net/gphttp/gpwebsocket"
)

type logEntryRange struct {
	Start, End int
}

type memLogger struct {
	*bytes.Buffer
	sync.RWMutex
	notifyLock sync.RWMutex
	connChans  *xsync.Map[chan *logEntryRange, struct{}]
	listeners  *xsync.Map[chan []byte, struct{}]
}

type MemLogger io.Writer

const (
	maxMemLogSize         = 16 * 1024
	truncateSize          = maxMemLogSize / 2
	initialWriteChunkSize = 4 * 1024
)

var memLoggerInstance = &memLogger{
	Buffer:    bytes.NewBuffer(make([]byte, maxMemLogSize)),
	connChans: xsync.NewMap[chan *logEntryRange, struct{}](),
	listeners: xsync.NewMap[chan []byte, struct{}](),
}

func GetMemLogger() MemLogger {
	return memLoggerInstance
}

func Handler() http.Handler {
	return memLoggerInstance
}

func HandlerFunc() http.HandlerFunc {
	return memLoggerInstance.ServeHTTP
}

func Events() (<-chan []byte, func()) {
	return memLoggerInstance.events()
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

func (m *memLogger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := gpwebsocket.Initiate(w, r)
	if err != nil {
		return
	}

	logCh := make(chan *logEntryRange)
	m.connChans.Store(logCh, struct{}{})

	defer func() {
		_ = conn.Close()

		m.notifyLock.Lock()
		m.connChans.Delete(logCh)
		close(logCh)
		m.notifyLock.Unlock()
	}()

	if err := m.wsInitial(conn); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	m.wsStreamLog(r.Context(), conn, logCh)
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
	if m.connChans.Size() == 0 && m.listeners.Size() == 0 {
		return
	}

	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()

	m.notifyLock.RLock()
	defer m.notifyLock.RUnlock()

	m.connChans.Range(func(ch chan *logEntryRange, _ struct{}) bool {
		select {
		case ch <- &logEntryRange{pos, pos + n}:
			return true
		case <-timeout.C:
			return false
		}
	})

	if m.listeners.Size() > 0 {
		msg := m.Bytes()[pos : pos+n]
		m.listeners.Range(func(ch chan []byte, _ struct{}) bool {
			select {
			case <-timeout.C:
				return false
			case ch <- msg:
				return true
			}
		})
	}
}

func (m *memLogger) writeBuf(b []byte) (pos int, err error) {
	m.Lock()
	defer m.Unlock()
	pos = m.Len()
	_, err = m.Buffer.Write(b)
	return
}

func (m *memLogger) events() (logs <-chan []byte, cancel func()) {
	ch := make(chan []byte)
	m.notifyLock.Lock()
	defer m.notifyLock.Unlock()
	m.listeners.Store(ch, struct{}{})

	return ch, func() {
		m.notifyLock.Lock()
		defer m.notifyLock.Unlock()
		m.listeners.Delete(ch)
		close(ch)
	}
}

func (m *memLogger) writeBytes(conn *websocket.Conn, b []byte) error {
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, b)
}

func (m *memLogger) wsInitial(conn *websocket.Conn) error {
	m.Lock()
	defer m.Unlock()

	return m.writeBytes(conn, m.Bytes())
}

func (m *memLogger) wsStreamLog(ctx context.Context, conn *websocket.Conn, ch <-chan *logEntryRange) {
	for {
		select {
		case <-ctx.Done():
			return
		case logRange := <-ch:
			m.RLock()
			msg := m.Bytes()[logRange.Start:logRange.End]
			err := m.writeBytes(conn, msg)
			m.RUnlock()
			if err != nil {
				return
			}
		}
	}
}
