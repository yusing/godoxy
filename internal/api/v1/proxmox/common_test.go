package proxmoxapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/proxmox"
	gpwebsocket "github.com/yusing/goutils/http/websocket"
	"github.com/yusing/goutils/task"
)

func newNodeLookupTestContext(t *testing.T, nodes *proxmox.NodePool) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	taskCtx := task.GetTestTask(t)
	if nodes != nil {
		proxmox.SetCtx(taskCtx, nodes)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(taskCtx.Context())
	return c, recorder
}

func TestNodeFromRequestReturnsUniqueNode(t *testing.T) {
	nodes := proxmox.NewNodePool()
	client := proxmox.NewClient("https://pve.example/api2/json")
	node := proxmox.NewNode(client, "pve", "node/pve")
	nodes.Add(node)
	c, recorder := newNodeLookupTestContext(t, nodes)

	got, ok := nodeFromRequest(c, "pve")
	require.True(t, ok)
	require.Same(t, node, got)
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestNodeFromRequestReportsMissingAndAmbiguousNames(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		c, recorder := newNodeLookupTestContext(t, proxmox.NewNodePool())
		_, ok := nodeFromRequest(c, "missing")
		require.False(t, ok)
		require.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("ambiguous", func(t *testing.T) {
		nodes := proxmox.NewNodePool()
		nodes.Add(proxmox.NewNode(proxmox.NewClient("https://one.example/api2/json"), "pve", "node/pve"))
		nodes.Add(proxmox.NewNode(proxmox.NewClient("https://two.example/api2/json"), "pve", "node/pve"))
		c, recorder := newNodeLookupTestContext(t, nodes)
		_, ok := nodeFromRequest(c, "pve")
		require.False(t, ok)
		require.Equal(t, http.StatusConflict, recorder.Code)
	})
}

func TestNodeFromRequestReportsMissingContextAsInternalError(t *testing.T) {
	c, _ := newNodeLookupTestContext(t, nil)
	_, ok := nodeFromRequest(c, "pve")
	require.False(t, ok)
	require.Len(t, c.Errors, 1)
}

func TestStreamProxmoxWebSocketFramesTextLines(t *testing.T) {
	handlerErrors := make(chan int, 1)
	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		streamProxmoxWebSocket(
			c,
			func(context.Context) (io.ReadCloser, error) {
				reader := iotest.OneByteReader(strings.NewReader("first line\nsecond line\n"))
				return io.NopCloser(reader), nil
			},
			(*gpwebsocket.Manager).CopyTextLines,
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

	for _, want := range []string{"first line\n", "second line\n"} {
		messageType, data, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, messageType)
		require.Equal(t, want, string(data))
	}

	select {
	case numErrors := <-handlerErrors:
		require.Zero(t, numErrors)
	case <-time.After(time.Second):
		t.Fatal("stream handler did not return after EOF")
	}
}
