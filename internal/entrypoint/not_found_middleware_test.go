package entrypoint

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/goutils/task"
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
	// The entrypoint hands a request copy (with the access log slot) to its
	// handlers, so check the address the access log saw, not the caller's
	// request object.
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

func TestConfigValidateRejectsNotFoundMiddlewareInResponsePhase(t *testing.T) {
	tests := []struct {
		name      string
		rules     string
		wantError bool
	}{
		{
			name: "request phase",
			rules: `default {
				middleware CloudflareRealIP {
				}
			}`,
		},
		{
			name: "response phase",
			rules: `status 404 {
				middleware CloudflareRealIP {
				}
			}`,
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var notFoundRules rules.Rules
			require.NoError(t, notFoundRules.Parse(test.rules))

			cfg := Config{}
			cfg.Rules.NotFound = notFoundRules
			err := cfg.Validate()
			if test.wantError {
				require.ErrorContains(t, err, "request-only actions (authentication or middleware) cannot be used in a response-phase rule or action block")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// With cloudflare_real_ip before cidr_whitelist, the log shows the visitor, the peer
// (cloudflared on loopback) and why the request was denied.
func TestAccessLogRecordsPeerIPAndDenial(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	server := newTestHTTPServer(t, ep)
	cfg := accesslog.DefaultRequestLoggerConfig()
	cfg.Format = accesslog.FormatJSON
	cfg.Path = filepath.Join(t.TempDir(), "access.log")
	logger, err := accesslog.NewAccessLogger(task.GetTestTask(t), cfg)
	require.NoError(t, err)
	ep.accessLogger = logger
	require.NoError(t, ep.SetMiddlewares([]map[string]any{
		{"use": "cloudflare_real_ip"},
		{"use": "cidr_whitelist", "allow": []string{"10.0.0.0/8"}},
	}))
	server.AddRoute(newFakeHTTPRoute(t, "app", ""))

	request := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("CF-Connecting-IP", "198.51.100.10")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
	logger.Flush()
	data, err := os.ReadFile(cfg.Path)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	require.Equal(t, "198.51.100.10", fields["ip"])
	require.Equal(t, "127.0.0.1", fields["peer_ip"])
	require.Equal(t, "CIDRWhitelist", fields["denied_by"])
	require.Equal(t, "IP not allowed", fields["deny_reason"])
}
