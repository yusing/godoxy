package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func TestWithConsumedRouteOverlaysPreservesExistingRequestContext(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	req = routes.WithRouteContext(req, fakeMiddlewareHTTPRoute{name: "test-route"})

	req = WithConsumedRouteOverlays(req, map[string]struct{}{
		"redirecthttp": {},
	}, map[string]struct{}{
		"oidc": {},
	})

	expect.Equal(t, routes.TryGetUpstreamName(req), "test-route")
	expect.True(t, isRouteBypassPromoted(req, "redirectHTTP"))
	expect.True(t, isRouteMiddlewareConsumed(req, "oidc"))
	expect.False(t, isRouteBypassPromoted(req, "forwardauth"))
	expect.False(t, isRouteMiddlewareConsumed(req, "forwardauth"))
}

func TestWithConsumedRouteOverlaysReturnsNewRequestWhenOverlayIsPresent(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com", nil)
	updated := WithConsumedRouteOverlays(req, map[string]struct{}{"redirecthttp": {}}, nil)

	expect.NotEqual(t, req, updated)
	expect.True(t, isRouteBypassPromoted(updated, "redirectHTTP"))
	expect.False(t, isRouteBypassPromoted(req, "redirectHTTP"))
}

type fakeMiddlewareHTTPRoute struct {
	name string
}

func (r fakeMiddlewareHTTPRoute) Key() string                                 { return r.name }
func (r fakeMiddlewareHTTPRoute) Name() string                                { return r.name }
func (r fakeMiddlewareHTTPRoute) Start(task.Parent) error                     { return nil }
func (r fakeMiddlewareHTTPRoute) Task() *task.Task                            { return nil }
func (r fakeMiddlewareHTTPRoute) Finish(any)                                  {}
func (r fakeMiddlewareHTTPRoute) MarshalZerologObject(*zerolog.Event)         {}
func (r fakeMiddlewareHTTPRoute) ProviderName() string                        { return "" }
func (r fakeMiddlewareHTTPRoute) GetProvider() types.RouteProvider            { return nil }
func (r fakeMiddlewareHTTPRoute) ListenURL() *nettypes.URL                    { return nil }
func (r fakeMiddlewareHTTPRoute) TargetURL() *nettypes.URL                    { return nil }
func (r fakeMiddlewareHTTPRoute) HealthMonitor() types.HealthMonitor          { return nil }
func (r fakeMiddlewareHTTPRoute) SetHealthMonitor(types.HealthMonitor)        {}
func (r fakeMiddlewareHTTPRoute) References() []string                        { return nil }
func (r fakeMiddlewareHTTPRoute) ShouldExclude() bool                         { return false }
func (r fakeMiddlewareHTTPRoute) Started() <-chan struct{}                    { return nil }
func (r fakeMiddlewareHTTPRoute) IdlewatcherConfig() *types.IdlewatcherConfig { return nil }
func (r fakeMiddlewareHTTPRoute) HealthCheckConfig() types.HealthCheckConfig {
	return types.HealthCheckConfig{}
}
func (r fakeMiddlewareHTTPRoute) LoadBalanceConfig() *types.LoadBalancerConfig {
	return nil
}
func (r fakeMiddlewareHTTPRoute) HomepageItem() homepage.Item                  { return homepage.Item{} }
func (r fakeMiddlewareHTTPRoute) DisplayName() string                          { return r.name }
func (r fakeMiddlewareHTTPRoute) ContainerInfo() *types.Container              { return nil }
func (r fakeMiddlewareHTTPRoute) InboundMTLSProfileRef() string                { return "" }
func (r fakeMiddlewareHTTPRoute) RouteMiddlewares() map[string]types.LabelMap  { return nil }
func (r fakeMiddlewareHTTPRoute) GetAgent() *agentpool.Agent                   { return nil }
func (r fakeMiddlewareHTTPRoute) IsDocker() bool                               { return false }
func (r fakeMiddlewareHTTPRoute) IsAgent() bool                                { return false }
func (r fakeMiddlewareHTTPRoute) UseLoadBalance() bool                         { return false }
func (r fakeMiddlewareHTTPRoute) UseIdleWatcher() bool                         { return false }
func (r fakeMiddlewareHTTPRoute) UseHealthCheck() bool                         { return false }
func (r fakeMiddlewareHTTPRoute) UseAccessLog() bool                           { return false }
func (r fakeMiddlewareHTTPRoute) ServeHTTP(http.ResponseWriter, *http.Request) {}
