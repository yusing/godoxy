package entrypoint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/types"
)

func TestSetMiddlewaresInvalidatesRouteOverlayCache(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	srv := newTestHTTPServer(t, ep)
	route := newFakeHTTPRoute(t, "test-route", "")
	route.routeMiddlewares = map[string]types.LabelMap{
		"redirectHTTP": {
			"bypass": "- path /health\n",
		},
	}
	route.handler = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	srv.AddRoute(route)

	require.NoError(t, ep.SetMiddlewares([]map[string]any{{
		"use": "redirectHTTP",
	}}))

	first := httptest.NewRecorder()
	srv.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "http://test-route/private", nil))
	require.Equal(t, http.StatusPermanentRedirect, first.Code)

	require.NoError(t, ep.SetMiddlewares([]map[string]any{{
		"use": "response",
		"set_headers": map[string]string{
			"X-Overlay-Reloaded": "true",
		},
	}}))

	second := httptest.NewRecorder()
	srv.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "http://test-route/private", nil))
	require.Equal(t, http.StatusNoContent, second.Code)
	require.Equal(t, "true", second.Header().Get("X-Overlay-Reloaded"))
}

func TestServeHTTPHidesEntrypointOverlayCompilationErrors(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	srv := newTestHTTPServer(t, ep)
	route := newFakeHTTPRoute(t, "test-route", "")
	route.routeMiddlewares = map[string]types.LabelMap{
		"redirectHTTP": {
			"bypass": "not-a-valid-bypass",
		},
	}
	route.handler = func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
	srv.AddRoute(route)

	require.NoError(t, ep.SetMiddlewares([]map[string]any{{
		"use": "redirectHTTP",
	}}))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://test-route/", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "internal server error\n", rec.Body.String())
}
