package routeimpl_test

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/health"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routeimpl"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/task"
)

const streamShutdownWaitLimit = 2 * time.Second

func TestStreamRouteCancelFinishesTask(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scheme route.Scheme
		listen func(t *testing.T) (addr string, close func())
		rebind func(t *testing.T, addr string) (close func())
	}{
		{
			name:   "tcp",
			scheme: route.SchemeTCP,
			listen: func(t *testing.T) (string, func()) {
				t.Helper()
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				return ln.Addr().String(), func() { _ = ln.Close() }
			},
			rebind: func(t *testing.T, addr string) func() {
				t.Helper()
				ln, err := net.Listen("tcp", addr)
				require.NoError(t, err)
				return func() { _ = ln.Close() }
			},
		},
		{
			name:   "udp",
			scheme: route.SchemeUDP,
			listen: func(t *testing.T) (string, func()) {
				t.Helper()
				ln, err := net.ListenPacket("udp", "127.0.0.1:0")
				require.NoError(t, err)
				return ln.LocalAddr().String(), func() { _ = ln.Close() }
			},
			rebind: func(t *testing.T, addr string) func() {
				t.Helper()
				ln, err := net.ListenPacket("udp", addr)
				require.NoError(t, err)
				return func() { _ = ln.Close() }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parent := task.GetTestTask(t).Subtask(tt.name+"-runtime", true)
			ep := entrypoint.NewEntrypoint(parent, nil)
			entrypoint.SetCtx(parent, ep)

			upstreamAddr, closeUpstream := tt.listen(t)
			t.Cleanup(closeUpstream)

			lisURL, err := nettypes.ParseURL(tt.name + "://127.0.0.1:0")
			require.NoError(t, err)
			proxyURL, err := nettypes.ParseURL(tt.name + "://" + upstreamAddr)
			require.NoError(t, err)

			impl, err := routeimpl.NewStreamRoute(&route.Route{
				Alias:       "echo-" + tt.name,
				Scheme:      tt.scheme,
				HealthCheck: health.HealthCheckConfig{Disable: true},
				Metadata: route.Metadata{
					LisURL:   lisURL,
					ProxyURL: proxyURL,
				},
			})
			require.NoError(t, err)
			require.NoError(t, impl.Start(parent))

			stream, ok := impl.(interface{ LocalAddr() net.Addr })
			require.True(t, ok)
			addr := stream.LocalAddr()
			require.NotNil(t, addr)

			started := time.Now()
			parent.FinishAndWait("test done")
			require.Less(t, time.Since(started), streamShutdownWaitLimit)

			closeRebound := tt.rebind(t, addr.String())
			closeRebound()
		})
	}
}

func TestStreamRouteReloadRebindsListenPort(t *testing.T) {
	t.Parallel()

	for _, scheme := range []route.Scheme{route.SchemeTCP, route.SchemeUDP} {
		t.Run(scheme.String(), func(t *testing.T) {
			t.Parallel()

			parent := task.GetTestTask(t).Subtask(scheme.String()+"-reload", true)
			t.Cleanup(func() { parent.FinishAndWait("test done") })
			entrypoint.SetCtx(parent, entrypoint.NewEntrypoint(parent, nil))

			current := startValidatedStreamRoute(t, parent, "reload-"+scheme.String(), scheme, 0)
			listenPort := streamListenPort(t, current)

			const reloads = 10
			for i := range reloads {
				// EventHandler.Update: stop the old stream, then start the
				// replacement on the same listen address.
				current.FinishAndWait("route update")
				next := startValidatedStreamRoute(t, parent, "reload-"+scheme.String(), scheme, listenPort)
				require.Equal(t, listenPort, streamListenPort(t, next), "reload %d rebound a different port", i)
				current = next
			}
		})
	}
}

func TestStreamRouteListenErrorFinishesTask(t *testing.T) {
	t.Parallel()

	parent := task.GetTestTask(t).Subtask("listen-error", true)
	ep := entrypoint.NewEntrypoint(parent, nil)
	entrypoint.SetCtx(parent, ep)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	lisURL, err := nettypes.ParseURL("tcp://" + ln.Addr().String())
	require.NoError(t, err)
	proxyURL, err := nettypes.ParseURL("tcp://127.0.0.1:1")
	require.NoError(t, err)

	impl, err := routeimpl.NewStreamRoute(&route.Route{
		Alias:       "busy-tcp",
		Scheme:      route.SchemeTCP,
		HealthCheck: health.HealthCheckConfig{Disable: true},
		Metadata: route.Metadata{
			LisURL:   lisURL,
			ProxyURL: proxyURL,
		},
	})
	require.NoError(t, err)
	require.Error(t, impl.Start(parent))

	started := time.Now()
	parent.FinishAndWait("test done")
	require.Less(t, time.Since(started), streamShutdownWaitLimit)
}

func TestStreamRouteMissingListenURL(t *testing.T) {
	t.Parallel()

	parent := task.GetTestTask(t).Subtask("missing-listen", true)
	ep := entrypoint.NewEntrypoint(parent, nil)
	entrypoint.SetCtx(parent, ep)

	impl, err := routeimpl.NewStreamRoute(&route.Route{
		Alias:       "no-listen",
		Scheme:      route.SchemeTCP,
		HealthCheck: health.HealthCheckConfig{Disable: true},
	})
	require.NoError(t, err)
	require.EqualError(t, impl.Start(parent), "listen URL is not set")

	started := time.Now()
	parent.FinishAndWait("test done")
	require.Less(t, time.Since(started), streamShutdownWaitLimit)
}

func startValidatedStreamRoute(t *testing.T, parent task.Parent, alias string, scheme route.Scheme, listenPort int) *route.Route {
	t.Helper()
	r := &route.Route{
		Alias:       alias,
		Scheme:      scheme,
		Host:        "127.0.0.1",
		Bind:        "127.0.0.1",
		Port:        route.Port{Proxy: 19001, Listening: listenPort},
		HealthCheck: health.HealthCheckConfig{Disable: true},
	}
	require.NoError(t, r.Validate())
	require.NoError(t, r.Start(parent), "listen %s://127.0.0.1:%d", scheme, listenPort)
	return r
}

func streamListenPort(t *testing.T, r *route.Route) int {
	t.Helper()
	stream, ok := r.Impl().(routing.StreamRoute)
	require.True(t, ok)
	addr := stream.LocalAddr()
	require.NotNil(t, addr)
	_, portStr, err := net.SplitHostPort(addr.String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portStr)
	require.NoError(t, err)
	require.NotZero(t, port)
	return port
}
