package memlogger

import (
	"bytes"
	"io"
	"slices"
	"sync"

	"github.com/puzpuzpuz/xsync/v4"
)

type memLogger struct {
	buf     *bytes.Buffer
	bufLock sync.RWMutex

	channelLock sync.RWMutex
	listeners   *xsync.Map[chan []byte, struct{}]
}

type MemLogger io.Writer

const (
	maxMemLogSize       = 16 * 1024
	truncateSize        = maxMemLogSize / 2
	listenerChanBufSize = 64
)

var memLoggerInstance = &memLogger{
	buf:       bytes.NewBuffer(make([]byte, 0, maxMemLogSize)),
	listeners: xsync.NewMap[chan []byte, struct{}](),
}

func GetMemLogger() MemLogger {
	return memLoggerInstance
}

func Events() (<-chan []byte, func()) {
	return memLoggerInstance.events()
}

// Write implements io.Writer.
func (m *memLogger) Write(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		return 0, nil
	}

	m.truncateIfNeeded(n)

	err = m.writeBuf(p)
	if err != nil {
		// not logging the error here, it will cause Run to be called again = infinite loop
		return n, err
	}

	if m.listeners.Size() == 0 {
		return n, nil
	}

	msg := slices.Clone(p)
	m.notifyWS(msg)
	return n, nil
}

func (m *memLogger) truncateIfNeeded(n int) {
	m.bufLock.RLock()
	needTruncate := m.buf.Len()+n > maxMemLogSize
	m.bufLock.RUnlock()

	if !needTruncate {
		return
	}

	m.bufLock.Lock()
	defer m.bufLock.Unlock()

	discard := m.buf.Len() - truncateSize
	if discard > 0 {
		_ = m.buf.Next(discard)
	}
}

func (m *memLogger) notifyWS(msg []byte) {
	if len(msg) == 0 || m.listeners.Size() == 0 {
		return
	}

	m.channelLock.RLock()
	defer m.channelLock.RUnlock()

	for ch := range m.listeners.Range {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (m *memLogger) writeBuf(b []byte) (err error) {
	m.bufLock.Lock()
	defer m.bufLock.Unlock()

	_, err = m.buf.Write(b)
	if err != nil {
		return err
	}

	if m.buf.Len() > maxMemLogSize {
		discard := m.buf.Len() - maxMemLogSize
		if discard > 0 {
			_ = m.buf.Next(discard)
		}
	}

	return nil
}

func (m *memLogger) events() (logs <-chan []byte, cancel func()) {
	ch := make(chan []byte, listenerChanBufSize)
	m.channelLock.Lock()
	defer m.channelLock.Unlock()
	m.listeners.Store(ch, struct{}{})

	return ch, func() {
		m.channelLock.Lock()
		defer m.channelLock.Unlock()
		m.listeners.Delete(ch)
		close(ch)
	}
}
