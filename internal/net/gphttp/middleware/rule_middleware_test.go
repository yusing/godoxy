package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/net/gphttp"
	"github.com/yusing/godoxy/internal/route/rules"
)

func TestRuleMiddlewareResolverSupportsRequestPhase(t *testing.T) {
	var configured rules.Rules
	require.NoError(t, configured.Parse(`
default {
	middleware CloudflareRealIP
}
`))
}

func TestRuleMiddlewareResolverSupportsRequestCompose(t *testing.T) {
	component, err := CloudflareRealIP.New(nil)
	require.NoError(t, err)

	const name = "testrulerequestcompose"
	previous, existed := allMiddlewares[name]
	allMiddlewares[name] = NewMiddlewareChain(name, []*Middleware{component})
	t.Cleanup(func() {
		if existed {
			allMiddlewares[name] = previous
		} else {
			delete(allMiddlewares, name)
		}
	})

	var configured rules.Rules
	require.NoError(t, configured.Parse(`
default {
	middleware testrulerequestcompose
}
`))
}

func TestRuleMiddlewareResolverRejectsUnsupportedMiddleware(t *testing.T) {
	responseWithBypass, err := ModifyResponse.New(OptionsRaw{
		"bypass": []string{"path /health"},
	})
	require.NoError(t, err)

	const wrappedResponseName = "testrulewrappedresponse"
	const responseComposeName = "testruleresponsecompose"
	allMiddlewares[wrappedResponseName] = responseWithBypass
	allMiddlewares[responseComposeName] = NewMiddlewareChain(responseComposeName, []*Middleware{responseWithBypass})
	t.Cleanup(func() {
		delete(allMiddlewares, wrappedResponseName)
		delete(allMiddlewares, responseComposeName)
	})

	const emptyComposeName = "testruleemptycompose"
	previous, existed := allMiddlewares[emptyComposeName]
	allMiddlewares[emptyComposeName] = NewMiddlewareChain(emptyComposeName, nil)
	t.Cleanup(func() {
		if existed {
			allMiddlewares[emptyComposeName] = previous
		} else {
			delete(allMiddlewares, emptyComposeName)
		}
	})

	tests := []struct {
		name       string
		middleware string
		errorText  string
	}{
		{name: "response only", middleware: "ModifyResponse", errorText: "has no request phase"},
		{name: "empty compose", middleware: emptyComposeName, errorText: "has no request phase"},
		{name: "wrapped response only", middleware: wrappedResponseName, errorText: "has no request phase"},
		{name: "response-only compose", middleware: responseComposeName, errorText: "has no request phase"},
		{name: "unknown", middleware: "does-not-exist", errorText: "unknown middleware"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var configured rules.Rules
			err := configured.Parse("default {\n  middleware " + test.middleware + "\n}")
			require.ErrorContains(t, err, test.errorText)
		})
	}
}

func TestRuleMiddlewareResolverAppliesBlockProperties(t *testing.T) {
	var configured rules.Rules
	require.NoError(t, configured.Parse(`
default {
	middleware RealIP {
		header: X-Forwarded-For
		from:
			- 127.0.0.1/32
	}
}
`))

	request := httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.10")
	response := httptest.NewRecorder()
	configured.BuildHandler(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}).ServeHTTP(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, "198.51.100.10:1234", request.RemoteAddr)
	require.Equal(t, "198.51.100.10", request.Header.Get("X-Real-IP"))
}

func TestRuleMiddlewareResolverPreservesNonUserAuthBypass(t *testing.T) {
	var configured rules.Rules
	require.NoError(t, configured.Parse(`
default {
	middleware CIDRWhitelist {
	}
}
`))

	t.Run("non-user request bypasses access control", func(t *testing.T) {
		fallbackCalled := false
		handler := configured.BuildHandler(func(w http.ResponseWriter, _ *http.Request) {
			fallbackCalled = true
			w.WriteHeader(http.StatusNoContent)
		})
		request := httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil)
		request.RemoteAddr = "203.0.113.10:1234"
		request = request.WithContext(gphttp.WithNonUserRequest(request.Context()))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.True(t, fallbackCalled)
		require.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("user request still applies access control", func(t *testing.T) {
		fallbackCalled := false
		handler := configured.BuildHandler(func(http.ResponseWriter, *http.Request) {
			fallbackCalled = true
		})
		request := httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil)
		request.RemoteAddr = "203.0.113.10:1234"
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		require.False(t, fallbackCalled)
		require.Equal(t, http.StatusForbidden, response.Code)
	})
}
