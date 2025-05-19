package gpwebsocket

import (
	"context"

	"github.com/gorilla/websocket"
)

type Writer struct {
	conn    *websocket.Conn
	msgType int
	ctx     context.Context
}

func NewWriter(ctx context.Context, conn *websocket.Conn, msgType int) *Writer {
	return &Writer{
		ctx:     ctx,
		conn:    conn,
		msgType: msgType,
	}
}

func (w *Writer) Write(p []byte) (int, error) {
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
		return len(p), w.conn.WriteMessage(w.msgType, p)
	}
}
