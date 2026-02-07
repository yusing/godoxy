package entrypoint_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yusing/godoxy/internal/common"
	. "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/goutils/task"
)

func TestShortLinkMatcher_FQDNAlias(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	matcher := ep.ShortLinkMatcher()
	matcher.AddRoute("app.domain.com")

	t.Run("exact path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.domain.com/", w.Header().Get("Location"))
	})

	t.Run("with path remainder", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app/foo/bar", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.domain.com/foo/bar", w.Header().Get("Location"))
	})

	t.Run("with query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app/foo?x=y&z=1", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.domain.com/foo?x=y&z=1", w.Header().Get("Location"))
	})
}

func TestShortLinkMatcher_SubdomainAlias(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	matcher := ep.ShortLinkMatcher()
	matcher.SetDefaultDomainSuffix(".example.com")
	matcher.AddRoute("app")

	t.Run("exact path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.example.com/", w.Header().Get("Location"))
	})

	t.Run("with path remainder", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app/foo/bar", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.example.com/foo/bar", w.Header().Get("Location"))
	})
}

func TestShortLinkMatcher_NotFound(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	matcher := ep.ShortLinkMatcher()
	matcher.SetDefaultDomainSuffix(".example.com")
	matcher.AddRoute("app")

	t.Run("missing key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown key", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/unknown", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestShortLinkMatcher_AddDelRoute(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	matcher := ep.ShortLinkMatcher()
	matcher.SetDefaultDomainSuffix(".example.com")

	matcher.AddRoute("app1")
	matcher.AddRoute("app2.domain.com")

	t.Run("both routes work", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app1", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app1.example.com/", w.Header().Get("Location"))

		req = httptest.NewRequest("GET", "/app2.domain.com", nil)
		w = httptest.NewRecorder()
		matcher.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app2.domain.com/", w.Header().Get("Location"))
	})

	t.Run("delete route", func(t *testing.T) {
		matcher.DelRoute("app1")

		req := httptest.NewRequest("GET", "/app1", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)

		req = httptest.NewRequest("GET", "/app2.domain.com", nil)
		w = httptest.NewRecorder()
		matcher.ServeHTTP(w, req)
		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app2.domain.com/", w.Header().Get("Location"))
	})
}

func TestShortLinkMatcher_NoDefaultDomainSuffix(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	matcher := ep.ShortLinkMatcher()
	// no SetDefaultDomainSuffix called

	t.Run("subdomain alias ignored", func(t *testing.T) {
		matcher.AddRoute("app")

		req := httptest.NewRequest("GET", "/app", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("FQDN alias still works", func(t *testing.T) {
		matcher.AddRoute("app.domain.com")

		req := httptest.NewRequest("GET", "/app.domain.com", nil)
		w := httptest.NewRecorder()
		matcher.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.domain.com/", w.Header().Get("Location"))
	})
}

func TestEntrypoint_ShortLinkDispatch(t *testing.T) {
	ep := NewEntrypoint(task.GetTestTask(t), nil)
	ep.ShortLinkMatcher().SetDefaultDomainSuffix(".example.com")
	ep.ShortLinkMatcher().AddRoute("app")

	server := NewHTTPServer(ep)
	err := server.Listen("localhost:0", HTTPProtoHTTP)
	require.NoError(t, err)

	t.Run("shortlink host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app", nil)
		req.Host = common.ShortLinkPrefix
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.example.com/", w.Header().Get("Location"))
	})

	t.Run("shortlink host with port", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app", nil)
		req.Host = common.ShortLinkPrefix + ":8080"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "https://app.example.com/", w.Header().Get("Location"))
	})

	t.Run("normal host", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/app", nil)
		req.Host = "app.example.com"
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		// Should not redirect, should try normal route lookup (which will 404)
		assert.NotEqual(t, http.StatusTemporaryRedirect, w.Code)
	})
}
