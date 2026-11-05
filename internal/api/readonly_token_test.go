package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/auth"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/goutils/task"
)

func TestReadOnlyBearerHandlers(t *testing.T) {
	t.Setenv("GODOXY_API_READONLY_TOKEN", "machine-test-credential")
	handler := newAuthenticatedHandler(t)
	parent := task.GetTestTask(t)
	ep := entrypoint.NewEntrypoint(parent, nil)
	entrypoint.SetCtx(parent, ep)
	ep.ExcludedRoutes().Add(&route.Route{Alias: "app"})
	preserveRuleHandlers(t)
	require.NoError(t, RegisterHandlers())
	for name, h := range map[string]http.Handler{"standalone": handler, "embedded": embeddedWebUIHandler(t)} {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/api/v1/route/list", "/api/v1/route/app"} {
				req := httptest.NewRequest(http.MethodGet, path, nil).WithContext(parent.Context())
				req.Header.Set("Authorization", "bEaReR machine-test-credential")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				require.Equal(t, http.StatusOK, rec.Code)
				assert.Contains(t, rec.Body.String(), `"alias":"app"`)
				assert.Empty(t, rec.Result().Cookies())
				assert.NotContains(t, rec.Body.String(), "machine-test-credential")
			}
		})
	}

	// A newly registered endpoint must not inherit token access, even under /route.
	called := false
	handler.GET("/api/v1/route/future", AuthMiddleware(), CSRFMiddleware(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	session := &http.Cookie{Name: "godoxy_token", Value: issueSessionToken(t)}
	for _, tc := range []struct {
		name, method, path string
		authorization      []string
		upgrade            bool
		status             int
	}{
		{"invalid with session", "GET", "/api/v1/route/list", []string{"Bearer wrong"}, false, 401},
		{"missing credential", "GET", "/api/v1/route/list", []string{"Bearer"}, false, 401},
		{"empty header", "GET", "/api/v1/route/list", []string{""}, false, 401},
		{"unsupported scheme", "GET", "/api/v1/route/list", []string{"Basic machine-test-credential"}, false, 401},
		{"malformed whitespace", "GET", "/api/v1/route/list", []string{"Bearer  machine-test-credential"}, false, 401},
		{"duplicate credentials", "GET", "/api/v1/route/list", []string{"Bearer machine-test-credential", "Bearer wrong"}, false, 401},
		{"write with session", "POST", "/api/v1/docker/start", []string{"Bearer machine-test-credential"}, false, 403},
		{"mutating GET", "GET", "/api/v1/cert/renew", []string{"Bearer machine-test-credential"}, false, 403},
		{"read upgrade", "GET", "/api/v1/route/list", []string{"Bearer machine-test-credential"}, true, 403},
		{"static sibling", "GET", "/api/v1/route/validate", []string{"Bearer machine-test-credential"}, false, 403},
		{"sensitive file read", "GET", "/api/v1/file/content", []string{"Bearer machine-test-credential"}, false, 403},
		{"session logout", "POST", "/api/v1/auth/logout", []string{"Bearer machine-test-credential"}, false, 403},
		{"future endpoint", "GET", "/api/v1/route/future", []string{"Bearer machine-test-credential"}, false, 403},
		{"unknown endpoint", "GET", "/api/v1/future", []string{"Bearer machine-test-credential"}, false, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil).WithContext(parent.Context())
			req.Header["Authorization"] = tc.authorization
			req.AddCookie(session)
			if tc.upgrade {
				req.Header.Set("Connection", "keep-alive, Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tc.status, rec.Code)
			assert.Empty(t, rec.Result().Cookies())
			assert.NotContains(t, rec.Body.String(), "machine-test-credential")
		})
	}
	assert.False(t, called)

	// Cookie-only requests retain their broader session and CSRF policy.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/route/future", nil)
	req.AddCookie(session)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, called)
	assert.NotNil(t, findCookie(rec.Result().Cookies(), auth.CSRFCookieName))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/docker/start", nil)
	req.AddCookie(session)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "missing CSRF token")
}

func TestReadOnlyBearerConfigurationBoundaries(t *testing.T) {
	for _, name := range []string{"GODOXY_API_READONLY_TOKEN", "GOPROXY_API_READONLY_TOKEN", "API_READONLY_TOKEN"} {
		t.Setenv(name, "")
	}
	handler := newAuthenticatedHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/route/list", nil)
	req.Header.Set("Authorization", "Bearer anything")
	req.AddCookie(&http.Cookie{Name: "godoxy_token", Value: issueSessionToken(t)})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	t.Setenv("GODOXY_API_READONLY_TOKEN", "machine-test-credential")
	local := NewHandler(false)
	common.DebugDisableAuth = true // restored by newAuthenticatedHandler cleanup
	disabled := NewHandler(true)
	for name, h := range map[string]http.Handler{"local": local, "disabled": disabled} {
		t.Run(name, func(t *testing.T) {
			// This non-allowlisted route still works, even with an invalid header.
			req := httptest.NewRequest(http.MethodGet, "/api/v1/route/providers", nil)
			req.Header.Set("Authorization", "Bearer wrong")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Empty(t, rec.Result().Cookies())
		})
	}
}

func TestReadOnlyBearerAccessLogRedaction(t *testing.T) {
	t.Setenv("GODOXY_API_READONLY_TOKEN", "machine-test-credential")
	_ = newAuthenticatedHandler(t)
	preserveRuleHandlers(t)
	require.NoError(t, RegisterHandlers())
	handler := embeddedWebUIHandler(t)
	parent := task.GetTestTask(t)
	entrypoint.SetCtx(parent, entrypoint.NewEntrypoint(parent, nil))
	cfg := accesslog.DefaultRequestLoggerConfig()
	cfg.Path = filepath.Join(t.TempDir(), "access.json")
	cfg.Format = accesslog.FormatJSON
	cfg.Fields.Headers.Default = accesslog.FieldModeKeep
	logger, err := accesslog.NewAccessLogger(parent, cfg)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, path string
		values     []string
		status     int
	}{
		{"accepted", "/api/v1/route/list", []string{"Bearer machine-test-credential"}, 200},
		{"redirect", "/api/v1/route/list/", []string{"Bearer machine-test-credential"}, 301},
		{"duplicate", "/api/v1/route/list", []string{"Bearer machine-test-credential", "Bearer rejected-credential"}, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(parent.Context())
			req.Header["Authorization"] = tc.values
			req.Header.Set("X-Audit", tc.name)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code)
			if tc.status == http.StatusMovedPermanently {
				assert.Equal(t, "/api/v1/route/list", rec.Header().Get("Location"))
			}
			// Match the entrypoint's post-handler logging of the original request.
			logger.LogRequest(req, rec.Result())
			assert.Equal(t, tc.values, req.Header.Values("Authorization"))
			logger.Flush()
			logged, err := os.ReadFile(cfg.Path)
			require.NoError(t, err)
			assert.Contains(t, string(logged), `"X-Audit":["`+tc.name+`"]`)
			assert.NotContains(t, string(logged), "machine-test-credential")
			assert.NotContains(t, string(logged), "rejected-credential")
		})
	}
}

func TestExplainAPIAuthentication(t *testing.T) {
	t.Setenv("GODOXY_API_READONLY_TOKEN", "machine-test-credential")
	h := newAuthenticatedHandler(t)
	parent := task.GetTestTask(t)
	entrypoint.SetCtx(parent, entrypoint.NewEntrypoint(parent, nil))
	for _, tc := range []struct {
		name   string
		cookie bool
		bearer bool
		want   int
	}{
		{"no session", false, false, 401}, {"session", true, false, 200}, {"read only", false, true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/route/explain?ip=127.0.0.1&host=app.example.com", nil).WithContext(parent.Context())
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: "godoxy_token", Value: issueSessionToken(t)})
			}
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer machine-test-credential")
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}
}
