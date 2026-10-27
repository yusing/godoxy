package metrics

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/goutils/http/websocket"
)

func TestGetAgentSystemInfoWithRetryPreservesCancellationCause(t *testing.T) {
	pool := agentpool.NewPool()
	cfg := &agent.AgentConfig{Addr: "127.0.0.1:1"}
	require.True(t, pool.Add(cfg))
	a, ok := pool.Get(cfg.Addr)
	require.True(t, ok)

	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("metrics subscriber disconnected")
	cancel(cause)

	data, err := getAgentSystemInfoWithRetry(ctx, a, "")
	require.ErrorIs(t, err, cause)
	retryErr := backoff.AsRetryError(err)
	require.NotNil(t, retryErr)
	require.ErrorIs(t, retryErr.LastErr, cause)
	require.Nil(t, data.RawMessage)
	require.Nil(t, data.release)
}

func TestQueueSystemInfoReleasesCancelledPayload(t *testing.T) {
	manager, _ := openMetricsSocket(t)
	manager.Close()
	released := 0
	queueSystemInfo(manager, make(chan systemInfoData), systemInfoData{
		systemInfo: bytesFromPool{release: func([]byte) { released++ }},
	})
	require.Equal(t, 1, released)
}

func TestStreamSystemInfoReleasesEveryPayload(t *testing.T) {
	for _, mode := range []string{"connected", "cancelled", "encode error"} {
		t.Run(mode, func(t *testing.T) {
			manager, conn := openMetricsSocket(t)
			if mode == "cancelled" {
				manager.Close()
			}
			dataCh := make(chan systemInfoData, 3)
			released := make([]int, 3)
			for i := range released {
				payload := []byte(`{"cpu":1}`)
				if mode == "encode error" && i == 0 {
					payload = []byte(`{`)
				}
				dataCh <- systemInfoData{agentName: "agent", systemInfo: bytesFromPool{
					RawMessage: payload,
					release:    func([]byte) { released[i]++ },
				}}
			}
			done := make(chan struct{})
			go func() {
				streamSystemInfo(manager, dataCh)
				close(done)
			}()
			close(dataCh)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("stream did not finish draining")
			}
			require.Equal(t, []int{1, 1, 1}, released)
			if mode == "connected" {
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
				for range released {
					_, payload, err := conn.ReadMessage()
					require.NoError(t, err)
					require.JSONEq(t, `{"agent":{"cpu":1}}`, string(payload))
				}
			}
		})
	}
}

func openMetricsSocket(t *testing.T) (*websocket.Manager, *gorilla.Conn) {
	t.Helper()
	managers := make(chan *websocket.Manager, 1)
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		manager, err := websocket.NewManagerWithUpgrade(c)
		if err != nil {
			t.Error(err)
			return
		}
		managers <- manager
		<-manager.Done()
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	conn, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	manager := <-managers
	t.Cleanup(func() {
		manager.Close()
		_ = conn.Close()
	})
	return manager, conn
}
