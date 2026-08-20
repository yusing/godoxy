package dockerapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
)

func TestStatsClosesStreamOnWebSocketDisconnect(t *testing.T) {
	const containerID = "stats-disconnect-test"
	streamStarted := make(chan struct{})
	streamCanceled := make(chan struct{})
	dockerDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("Api-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/"+containerID+"/stats"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"read":`))
			w.(http.Flusher).Flush()
			close(streamStarted)
			<-r.Context().Done()
			close(streamCanceled)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(dockerDaemon.Close)

	docker.RegisterContainerConfig(containerID, types.DockerProviderConfig{URL: dockerDaemon.URL})
	t.Cleanup(func() {
		docker.UnregisterContainerConfig(containerID)
	})

	handlerDone := make(chan struct{})
	router := gin.New()
	router.GET("/docker/stats/:id", func(c *gin.Context) {
		Stats(c)
		close(handlerDone)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/docker/stats/" + containerID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	select {
	case <-streamStarted:
	case <-time.After(time.Second):
		t.Fatal("Docker stats stream did not start")
	}

	deadline := time.Now().Add(time.Second)
	require.NoError(t, conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		deadline,
	))
	require.NoError(t, conn.Close())

	select {
	case <-streamCanceled:
	case <-time.After(time.Second):
		t.Fatal("Docker stats stream was not canceled after WebSocket disconnect")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("stats handler did not return after WebSocket disconnect")
	}
}
