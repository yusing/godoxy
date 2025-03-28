package gpwebsocket

import (
	"context"

	"github.com/coder/websocket"
)

type Writer struct {
	conn    *websocket.Conn
	msgType websocket.MessageType
	ctx     context.Context
}

func NewWriter(ctx context.Context, conn *websocket.Conn, msgType websocket.MessageType) *Writer {
	return &Writer{
		ctx:     ctx,
		conn:    conn,
		msgType: msgType,
	}
}

func (w *Writer) Write(p []byte) (int, error) {
	return len(p), w.conn.Write(w.ctx, w.msgType, p)
}

func (w *Writer) Close() error {
	return w.conn.CloseNow()
}
