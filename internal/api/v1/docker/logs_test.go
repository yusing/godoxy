package dockerapi

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/synk"
)

func TestLogsStreamsTTYLines(t *testing.T) {
	testLogsStream(
		t,
		true,
		[]byte("first line\nsecond line\nlast line"),
		[]string{"first line\n", "second line\n", "last line"},
	)
}

func TestLogsDemultiplexesNonTTYFrames(t *testing.T) {
	bufferPool := synk.GetUnsizedBytesPool()
	input := bufferPool.GetBuffer()
	defer bufferPool.PutBuffer(input)
	_, err := stdcopy.NewStdWriter(input, stdcopy.Stdout).Write([]byte("stdout line\n"))
	require.NoError(t, err)
	_, err = stdcopy.NewStdWriter(input, stdcopy.Stderr).Write([]byte("stderr line\n"))
	require.NoError(t, err)

	testLogsStream(t, false, input.Bytes(), []string{"stdout line\n", "stderr line\n"})
}

func TestLogsRejectsInvalidNonTTYFrame(t *testing.T) {
	invalidHeader := []byte{9, 0, 0, 0, 0, 0, 0, 0}
	testLogsStream(t, false, invalidHeader, nil)
}

func TestLogsClosesTTYStreamOnWebSocketDisconnect(t *testing.T) {
	const containerID = "disconnect-test"
	streamStarted := make(chan struct{})
	streamCanceled := make(chan struct{})
	dockerDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("Api-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/"+containerID+"/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"Config":{"Tty":true}}`))
		case strings.HasSuffix(r.URL.Path, "/containers/"+containerID+"/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(bytes.Repeat([]byte("x"), 8192))
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
	router.GET("/docker/logs/:id", func(c *gin.Context) {
		Logs(c)
		close(handlerDone)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/docker/logs/" + containerID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	select {
	case <-streamStarted:
	case <-time.After(time.Second):
		t.Fatal("Docker log stream did not start")
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
		t.Fatal("Docker log stream was not canceled after WebSocket disconnect")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("logs handler did not return after WebSocket disconnect")
	}
}

func testLogsStream(t *testing.T, tty bool, input []byte, want []string) {
	t.Helper()

	const containerID = "test"
	dockerDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_ping":
			w.Header().Set("Api-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/"+containerID+"/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"Config":{"Tty":%t}}`, tty)
		case strings.HasSuffix(r.URL.Path, "/containers/"+containerID+"/logs"):
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(input)
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
	router.GET("/docker/logs/:id", func(c *gin.Context) {
		Logs(c)
		close(handlerDone)
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/docker/logs/" + containerID
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	for _, expected := range want {
		messageType, data, err := conn.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, websocket.TextMessage, messageType)
		require.Equal(t, expected, string(data))
	}

	if len(want) == 0 {
		_, _, err := conn.ReadMessage()
		require.Error(t, err)
	}

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("logs handler did not return after input EOF")
	}
}
