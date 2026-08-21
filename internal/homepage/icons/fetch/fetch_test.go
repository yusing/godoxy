package iconfetch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage/icons"
	"github.com/yusing/godoxy/internal/net/gphttp"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/goutils/task"
)

func TestFindIconDoesNotScrapeUnlessHealthy(t *testing.T) {
	t.Parallel()

	for _, status := range []health.HealthStatus{
		health.StatusNapping,
		health.StatusStarting,
		health.StatusUnhealthy,
		health.StatusError,
		health.StatusUnknown,
	} {
		t.Run(status.String(), func(t *testing.T) {
			t.Parallel()
			r := newStubIconRoute(t)
			r.mon.status = status

			result, err := FindIcon(t.Context(), r, "/", icons.VariantNone)

			require.Error(t, err)
			require.Equal(t, http.StatusServiceUnavailable, result.StatusCode)
			require.Zero(t, r.served)
		})
	}
}

func TestFindIconScrapesHealthyRoute(t *testing.T) {
	r := newStubIconRoute(t)
	r.mon.status = health.StatusHealthy
	r.handler = func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "image/png")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("png-bytes"))
	}

	result, err := FindIcon(t.Context(), r, "/", icons.VariantNone)

	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), result.Icon)
	require.EqualValues(t, 1, r.served)
}

func TestFindIconFaviconHrefDoesNotRecurseIntoFindIcon(t *testing.T) {
	r := newStubIconRoute(t)
	r.mon.status = health.StatusHealthy
	r.handler = func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/favicon.ico" {
			require.True(t, gphttp.IsNonUserRequest(req.Context()), "favicon follow-up must stay on the non-user request context")
			rw.Header().Set("Content-Type", "image/png")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte("from-href"))
			return
		}
		rw.Header().Set("Content-Type", "text/html")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(`<html><head><link rel="icon" href="/favicon.ico"></head></html>`))
	}

	result, err := FindIcon(t.Context(), r, "/", icons.VariantNone)

	require.NoError(t, err)
	require.Equal(t, []byte("from-href"), result.Icon)
	require.EqualValues(t, 2, r.served)
}

func TestFindIconInstallsScrapedRouteContext(t *testing.T) {
	scrapedRoute := newStubIconRoute(t)
	scrapedRoute.mon.status = health.StatusHealthy
	scrapedRoute.handler = func(rw http.ResponseWriter, req *http.Request) {
		require.Same(t, scrapedRoute, routes.TryGetRoute(req))
		require.Equal(t, scrapedRoute.Name(), routes.TryGetUpstreamName(req))
		require.Equal(t, scrapedRoute.TargetURL().Host, routes.TryGetUpstreamHostPort(req))
		rw.Header().Set("Content-Type", "image/png")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte("png-bytes"))
	}

	callerRoute := newStubIconRoute(t)
	callerRoute.key = "caller"
	callerRoute.target = nettypes.NewURL(&url.URL{Scheme: "http", Host: "caller.test"})
	callerRequest := httptest.NewRequest(http.MethodGet, "http://dashboard.test/api/v1/favicon", nil)
	callerRequest = routes.WithRouteContext(callerRequest, callerRoute)

	result, err := FindIcon(callerRequest.Context(), scrapedRoute, "/", icons.VariantNone)

	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), result.Icon)
	require.EqualValues(t, 1, scrapedRoute.served)
}

type stubIconRoute struct {
	key     string
	target  *nettypes.URL
	mon     stubHealthMonitor
	handler http.HandlerFunc
	served  int
}

func newStubIconRoute(t *testing.T) *stubIconRoute {
	t.Helper()
	return &stubIconRoute{
		key:    t.Name(),
		target: nettypes.NewURL(&url.URL{Scheme: "http", Host: "icon.test"}),
	}
}

func (r *stubIconRoute) Key() string                         { return r.key }
func (r *stubIconRoute) Name() string                        { return r.key }
func (r *stubIconRoute) ProviderName() string                { return "test" }
func (r *stubIconRoute) References() []string                { return nil }
func (r *stubIconRoute) TargetURL() *nettypes.URL            { return r.target }
func (r *stubIconRoute) HealthMonitor() health.HealthMonitor { return r.mon }
func (r *stubIconRoute) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	r.served++
	if r.handler != nil {
		r.handler(rw, req)
	}
}

type stubHealthMonitor struct {
	status health.HealthStatus
}

func (m stubHealthMonitor) Start(task.Parent) error { return nil }
func (m stubHealthMonitor) Task() *task.Task        { return nil }
func (m stubHealthMonitor) Finish(any)              {}
func (m stubHealthMonitor) String() string          { return "stub" }
func (m stubHealthMonitor) Name() string            { return "stub" }
func (m stubHealthMonitor) Status() health.HealthStatus {
	return m.status
}
func (m stubHealthMonitor) Uptime() time.Duration  { return 0 }
func (m stubHealthMonitor) Latency() time.Duration { return 0 }
func (m stubHealthMonitor) Detail() string         { return "" }
func (m stubHealthMonitor) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.status.String())
}

var _ http.Handler = (*stubIconRoute)(nil)
