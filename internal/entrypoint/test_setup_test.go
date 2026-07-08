package entrypoint

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"

	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

// NewTestEntrypoint is a test-only bridge for external entrypoint_test tests.
func NewTestEntrypoint(tb testing.TB, cfg *Config) *Entrypoint {
	tb.Helper()

	testTask := task.GetTestTask(tb)
	ep := NewEntrypoint(testTask, cfg)
	SetCtx(testTask, ep)
	return ep
}

// NewHTTPServer is a test-only bridge for external entrypoint_test tests.
func NewHTTPServer(ep *Entrypoint) HTTPServer {
	return newHTTPServer(ep)
}

func mustParseURL(tb testing.TB, rawURL string) *nettypes.URL {
	tb.Helper()

	u, err := nettypes.ParseURL(rawURL)
	if err != nil {
		tb.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return u
}

type testHTTPRoute struct {
	*route.Route
}

func (r *testHTTPRoute) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (r *testHTTPRoute) Start(parent task.Parent) error {
	r.Route.Init(parent, "test."+r.Name(), false)
	ep, ok := routing.EntrypointFromCtx(parent.Context()).(*Entrypoint)
	if !ok {
		return nil
	}
	httpAddr, httpsAddr := getAddr(r)
	for _, addr := range []string{httpAddr, httpsAddr} {
		if addr == "" {
			continue
		}
		srv, _ := ep.servers.LoadOrCompute(addr, func() (*httpServer, bool) {
			srv := newHTTPServer(ep)
			srv.addr = addr
			srv.routes = pool.New[routing.HTTPRoute](fmt.Sprintf("[test] %s", addr), "http_routes")
			return srv, false
		})
		srv.AddRoute(r)
	}
	return nil
}

func TestMain(m *testing.M) {
	route.InitBuilder(func(r *route.Route) (routing.Route, *agentpool.Agent, error) {
		if r.Homepage == nil {
			r.Homepage = &homepage.ItemConfig{Name: r.Alias, Show: true}
		}
		if r.Host == "" {
			r.Host = "127.0.0.1"
		}
		if r.Scheme == route.SchemeNone {
			r.Scheme = route.SchemeHTTP
		}
		if r.Port.Proxy == 0 {
			r.Port.Proxy = 80
		}
		var err error
		r.LisURL, err = nettypes.ParseURL("https://" + net.JoinHostPort(r.Bind, strconv.Itoa(r.Port.Listening)))
		if err != nil {
			return nil, nil, err
		}
		r.ProxyURL, err = nettypes.ParseURL(fmt.Sprintf("%s://%s", r.Scheme, net.JoinHostPort(r.Host, strconv.Itoa(r.Port.Proxy))))
		if err != nil {
			return nil, nil, err
		}
		return &testHTTPRoute{Route: r}, nil, nil
	})
	os.Exit(m.Run())
}
