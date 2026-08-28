package entrypoint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
)

type remoteAddrAccessLogger struct {
	accesslog.AccessLogger
	remoteAddr string
}

func (logger *remoteAddrAccessLogger) LogRequest(request *http.Request, _ *http.Response) {
	logger.remoteAddr = request.RemoteAddr
}

func TestNotFoundRuleRequestMiddlewareUpdatesAccessLogAddress(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	server := newTestHTTPServer(t, ep)
	logger := &remoteAddrAccessLogger{}
	ep.accessLogger = logger

	var notFoundRules rules.Rules
	require.NoError(t, notFoundRules.Parse(`
default {
	middleware CloudflareRealIP {
	}
}
`))
	ep.SetNotFoundRules(notFoundRules)

	request := httptest.NewRequest(http.MethodGet, "http://unknown.example/garbage", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("CF-Connecting-IP", "198.51.100.10")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, "198.51.100.10:1234", request.RemoteAddr)
	require.Equal(t, "198.51.100.10:1234", logger.remoteAddr)
	require.Equal(t, "198.51.100.10", request.Header.Get("X-Real-IP"))
}

func TestNotFoundRuleRequestMiddlewareIsOptIn(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	server := newTestHTTPServer(t, ep)
	logger := &remoteAddrAccessLogger{}
	ep.accessLogger = logger

	request := httptest.NewRequest(http.MethodGet, "http://unknown.example/garbage", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("CF-Connecting-IP", "198.51.100.10")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, "127.0.0.1:1234", request.RemoteAddr)
	require.Equal(t, "127.0.0.1:1234", logger.remoteAddr)
}

func TestNotFoundRuleRequestMiddlewareDoesNotRunForMatchedRoute(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	server := newTestHTTPServer(t, ep)
	logger := &remoteAddrAccessLogger{}
	ep.accessLogger = logger

	matchedRoute := newFakeHTTPRoute(t, "matched.example", "")
	matchedRoute.handler = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	server.AddRoute(matchedRoute)

	var notFoundRules rules.Rules
	require.NoError(t, notFoundRules.Parse(`
default {
	middleware CloudflareRealIP {
	}
}
`))
	ep.SetNotFoundRules(notFoundRules)

	request := httptest.NewRequest(http.MethodGet, "http://matched.example/garbage", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("CF-Connecting-IP", "198.51.100.10")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "127.0.0.1:1234", request.RemoteAddr)
	require.Equal(t, "127.0.0.1:1234", logger.remoteAddr)
}
