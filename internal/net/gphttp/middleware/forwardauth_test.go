package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/routing"
)

type forwardAuthTestEntrypoint struct {
	routing.Entrypoint
	route routing.HTTPRoute
}

func (ep forwardAuthTestEntrypoint) HTTPRoutes() routing.PoolLike[routing.HTTPRoute] {
	return forwardAuthTestRoutes{route: ep.route}
}

type forwardAuthTestRoutes struct {
	route routing.HTTPRoute
}

func (routes forwardAuthTestRoutes) Get(alias string) (routing.HTTPRoute, bool) {
	if alias != routes.route.Name() {
		return nil, false
	}
	return routes.route, true
}

func (routes forwardAuthTestRoutes) Iter(yield func(alias string, route routing.HTTPRoute) bool) {
	yield(routes.route.Name(), routes.route)
}

func (forwardAuthTestRoutes) Size() int {
	return 1
}

func TestForwardAuthDoesNotForwardClientIdentityHeaders(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Remote-User", "authenticated-user")
		w.Header().Set("X-Auth-Role", "member")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer authServer.Close()

	forwardAuth, err := ForwardAuth.New(map[string]any{
		"route":         "auth",
		"auth_endpoint": "/",
	})
	require.NoError(t, err)

	ep := forwardAuthTestEntrypoint{route: fakeMiddlewareHTTPRoute{
		name:      "auth",
		targetURL: mustParseURL(authServer.URL),
	}}
	req := httptest.NewRequest(http.MethodGet, "http://upstream.example/protected", nil)
	req.Header.Set("Remote-User", "spoofed-user")
	req.Header.Set("Remote-Groups", "admins")
	req = req.WithContext(context.WithValue(req.Context(), routing.EntrypointContextKey{}, ep))

	upstreamCalled := false
	forwardAuth.ModifyRequest(func(_ http.ResponseWriter, upstreamReq *http.Request) {
		upstreamCalled = true
		assert.Equal(t, "authenticated-user", upstreamReq.Header.Get("Remote-User"))
		assert.Empty(t, upstreamReq.Header.Get("Remote-Groups"))
	}, httptest.NewRecorder(), req)

	assert.True(t, upstreamCalled)

	customForwardAuth, err := ForwardAuth.New(map[string]any{
		"route":         "auth",
		"auth_endpoint": "/",
		"headers":       []string{"X-Auth-Role", "X-Auth-Permissions"},
	})
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "http://upstream.example/protected", nil)
	req.Header.Set("Remote-Groups", "admins")
	req.Header.Set("X-Auth-Role", "admin")
	req.Header.Set("X-Auth-Permissions", "root")
	req = req.WithContext(context.WithValue(req.Context(), routing.EntrypointContextKey{}, ep))
	customUpstreamCalled := false
	customForwardAuth.ModifyRequest(func(_ http.ResponseWriter, upstreamReq *http.Request) {
		customUpstreamCalled = true
		assert.Empty(t, upstreamReq.Header.Get("Remote-Groups"))
		assert.Equal(t, "member", upstreamReq.Header.Get("X-Auth-Role"))
		assert.Empty(t, upstreamReq.Header.Get("X-Auth-Permissions"))
	}, httptest.NewRecorder(), req)
	assert.True(t, customUpstreamCalled)
}
