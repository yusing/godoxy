package websocket

import (
	"io"
	"time"
)

type Writer struct {
	msgType int
	manager *Manager
}

func (cm *Manager) NewWriter(msgType int) io.Writer {
	return &Writer{
		msgType: msgType,
		manager: cm,
	}
}

func (w *Writer) Write(p []byte) (int, error) {
	return len(p), w.manager.WriteData(w.msgType, p, 10*time.Second)
}
