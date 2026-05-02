package handler_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/agent/pkg/agentproxy"
	"github.com/yusing/godoxy/agent/pkg/handler"
	route "github.com/yusing/godoxy/internal/route/types"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestProxyHTTPH2C(t *testing.T) {
	gotBackendProto := make(chan int, 1)
	backend := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBackendProto <- r.ProtoMajor
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}), &http2.Server{}))
	backend.Start()
	t.Cleanup(backend.Close)

	backendURL, err := url.Parse(backend.URL)
	require.NoError(t, err)
	_, err = strconv.Atoi(backendURL.Port())
	require.NoError(t, err)

	req := httptest.NewRequest(
		http.MethodPost,
		"http://agent.local"+agent.APIEndpointBase+agent.EndpointProxyHTTP+"/management.ManagementService/GetServerKey",
		strings.NewReader("grpc-body"),
	)
	req.Header.Set("Content-Type", "application/grpc+proto")
	(&agentproxy.Config{
		Scheme:     "h2c",
		Host:       backendURL.Host,
		HTTPConfig: route.HTTPConfig{},
	}).SetAgentProxyConfigHeaders(req.Header)

	rec := httptest.NewRecorder()
	handler.ProxyHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "application/grpc", res.Header.Get("Content-Type"))
	require.Equal(t, "0", res.Trailer.Get("Grpc-Status"))

	select {
	case proto := <-gotBackendProto:
		require.Equal(t, 2, proto)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend request")
	}
}
