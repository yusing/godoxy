package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/yusing/go-proxy/internal/common"
)

// Manager handles WebSocket connection state and ping-pong
type Manager struct {
	conn             *websocket.Conn
	ctx              context.Context
	cancel           context.CancelFunc
	pongWriteTimeout time.Duration
	pingCheckTicker  *time.Ticker
	lastPingTime     atomic.Value
	readCh           chan []byte
	err              error

	writeLock sync.Mutex
}

var defaultUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// TODO: add CORS
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var (
	ErrReadTimeout  = errors.New("read timeout")
	ErrWriteTimeout = errors.New("write timeout")
)

const (
	TextMessage   = websocket.TextMessage
	BinaryMessage = websocket.BinaryMessage
)

// NewManagerWithUpgrade upgrades the HTTP connection to a WebSocket connection and returns a Manager.
// If the upgrade fails, the error is returned.
// If the upgrade succeeds, the Manager is returned.
func NewManagerWithUpgrade(c *gin.Context, upgrader ...websocket.Upgrader) (*Manager, error) {
	var actualUpgrader websocket.Upgrader
	if len(upgrader) == 0 {
		actualUpgrader = defaultUpgrader
	} else {
		actualUpgrader = upgrader[0]
	}

	conn, err := actualUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(c.Request.Context())
	cm := &Manager{
		conn:             conn,
		ctx:              ctx,
		cancel:           cancel,
		pongWriteTimeout: 2 * time.Second,
		pingCheckTicker:  time.NewTicker(3 * time.Second),
		readCh:           make(chan []byte, 1),
	}
	cm.lastPingTime.Store(time.Now())

	conn.SetCloseHandler(func(code int, text string) error {
		if common.IsDebug {
			cm.err = fmt.Errorf("connection closed: code=%d, text=%s", code, text)
		}
		cm.Close()
		return nil
	})

	go cm.pingCheckRoutine()
	go cm.readRoutine()

	return cm, nil
}

// Periodic writes data to the connection periodically.
// If the connection is closed, the error is returned.
// If the write timeout is reached, ErrWriteTimeout is returned.
func (cm *Manager) PeriodicWrite(interval time.Duration, getData func() (any, error)) error {
	write := func() {
		data, err := getData()
		if err != nil {
			cm.err = err
			cm.Close()
			return
		}

		if err := cm.WriteJSON(data, interval); err != nil {
			cm.err = err
			cm.Close()
		}
	}

	// initial write before the ticker starts
	write()
	if cm.err != nil {
		return cm.err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-cm.ctx.Done():
			return cm.err
		case <-ticker.C:
			write()
			if cm.err != nil {
				return cm.err
			}
		}
	}
}

// WriteJSON writes a JSON message to the connection with json.
// If the connection is closed, the error is returned.
// If the write timeout is reached, ErrWriteTimeout is returned.
func (cm *Manager) WriteJSON(data any, timeout time.Duration) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return cm.WriteData(websocket.TextMessage, bytes, timeout)
}

// WriteData writes a message to the connection with sonic.
// If the connection is closed, the error is returned.
// If the write timeout is reached, ErrWriteTimeout is returned.
func (cm *Manager) WriteData(typ int, data []byte, timeout time.Duration) error {
	select {
	case <-cm.ctx.Done():
		return cm.err
	default:
		cm.writeLock.Lock()
		defer cm.writeLock.Unlock()

		if err := cm.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}
		err := cm.conn.WriteMessage(typ, data)
		if err != nil {
			if errors.Is(err, websocket.ErrCloseSent) {
				return cm.err
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return ErrWriteTimeout
			}
			return err
		}
		return nil
	}
}

// ReadJSON reads a JSON message from the connection and unmarshals it into the provided struct with sonic
// If the connection is closed, the error is returned.
// If the message fails to unmarshal, the error is returned.
// If the read timeout is reached, ErrReadTimeout is returned.
func (cm *Manager) ReadJSON(out any, timeout time.Duration) error {
	select {
	case <-cm.ctx.Done():
		return cm.err
	case data := <-cm.readCh:
		return json.Unmarshal(data, out)
	case <-time.After(timeout):
		return ErrReadTimeout
	}
}

// Close closes the connection and cancels the context
func (cm *Manager) Close() {
	cm.cancel()
	cm.pingCheckTicker.Stop()
	cm.conn.Close()
}

func (cm *Manager) GracefulClose() {
	cm.writeLock.Lock()
	defer cm.writeLock.Unlock()

	_ = cm.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_ = cm.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

	cm.Close()
}

// Done returns a channel that is closed when the context is done or the connection is closed
func (cm *Manager) Done() <-chan struct{} {
	return cm.ctx.Done()
}

func (cm *Manager) pingCheckRoutine() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-cm.pingCheckTicker.C:
			if time.Since(cm.lastPingTime.Load().(time.Time)) > 5*time.Second {
				if common.IsDebug {
					cm.err = errors.New("no ping received in 5 seconds, closing connection")
				}
				cm.Close()
				return
			}
		}
	}
}

func (cm *Manager) readRoutine() {
	for {
		select {
		case <-cm.ctx.Done():
			return
		default:
			typ, data, err := cm.conn.ReadMessage()
			if err != nil {
				if cm.ctx.Err() == nil { // connection is not closed
					cm.err = fmt.Errorf("failed to read message: %w", err)
					cm.Close()
				}
				return
			}

			if typ == websocket.TextMessage && string(data) == "ping" {
				cm.lastPingTime.Store(time.Now())
				if err := cm.WriteData(websocket.TextMessage, []byte("pong"), cm.pongWriteTimeout); err != nil {
					cm.err = fmt.Errorf("failed to write pong message: %w", err)
					cm.Close()
					return
				}
				continue
			}

			if typ == websocket.TextMessage || typ == websocket.BinaryMessage {
				select {
				case <-cm.ctx.Done():
					return
				case cm.readCh <- data:
				}
			}
		}
	}
}
