package proxmoxapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/goutils/task"
)

type cancelAwareReader struct {
	ctx       context.Context
	closed    chan struct{}
	closeOnce sync.Once
}

func (r *cancelAwareReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, r.ctx.Err()
}

func (r *cancelAwareReader) Close() error {
	r.closeOnce.Do(func() {
		close(r.closed)
	})
	return nil
}

func TestStreamProxmoxWebSocketCloseCancelsAndClosesReader(t *testing.T) {
	readerOpened := make(chan *cancelAwareReader, 1)
	handlerErrors := make(chan int, 1)
	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		streamProxmoxWebSocket(
			c,
			func(ctx context.Context) (io.ReadCloser, error) {
				reader := &cancelAwareReader{
					ctx:    ctx,
					closed: make(chan struct{}),
				}
				readerOpened <- reader
				return reader, nil
			},
			"failed to open stream",
			"failed to copy stream",
		)
		handlerErrors <- len(c.Errors)
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/stream"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	var reader *cancelAwareReader
	select {
	case reader = <-readerOpened:
	case <-time.After(time.Second):
		t.Fatal("stream reader was not opened")
	}

	deadline := time.Now().Add(time.Second)
	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		deadline,
	))
	require.NoError(t, conn.Close())

	select {
	case <-reader.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("closing the WebSocket did not cancel the stream context")
	}
	select {
	case <-reader.closed:
	case <-time.After(time.Second):
		t.Fatal("stream reader was not closed")
	}
	select {
	case numErrors := <-handlerErrors:
		require.Zero(t, numErrors)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not return after WebSocket cancellation")
	}
}

func TestNodeStatsWebSocketCloseCancelsUpstreamRequest(t *testing.T) {
	upstreamStarted := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(upstreamStarted)
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	t.Cleanup(upstream.Close)

	client := proxmox.NewClient(upstream.URL, goproxmox.WithHTTPClient(upstream.Client()))
	node := proxmox.NewNode(client, "pve", "node/pve")
	nodes := proxmox.NewNodePool()
	nodes.Add(node)
	taskCtx := task.GetTestTask(t)
	proxmox.SetCtx(taskCtx, nodes)

	handlerErrors := make(chan int, 1)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(taskCtx.Context())
		c.Next()
		handlerErrors <- len(c.Errors)
	})
	router.GET("/proxmox/stats/:node", NodeStats)

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/proxmox/stats/pve"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		t.Fatal("node stats request did not reach Proxmox")
	}

	deadline := time.Now().Add(time.Second)
	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		deadline,
	))
	require.NoError(t, conn.Close())

	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("closing the stats WebSocket did not cancel the Proxmox request")
	}

	select {
	case numErrors := <-handlerErrors:
		require.Zero(t, numErrors)
	case <-time.After(time.Second):
		t.Fatal("stats handler did not return after WebSocket cancellation")
	}
}
