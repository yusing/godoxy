package entrypoint_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "github.com/yusing/godoxy/internal/entrypoint"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/route"
	routeTypes "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func addRoute(t *testing.T, alias string) {
	t.Helper()

	ep := entrypoint.FromCtx(task.GetTestTask(t).Context())
	require.NotNil(t, ep)

	_, err := route.NewStartedTestRoute(t, &route.Route{
		Alias:  alias,
		Scheme: routeTypes.SchemeHTTP,
		Port: route.Port{
			Listening: 1000,
			Proxy:     8080,
		},
		HealthCheck: types.HealthCheckConfig{
			Disable: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	route, ok := ep.HTTPRoutes().Get(alias)
	require.True(t, ok, "route not found")
	require.NotNil(t, route)
}

func run(t *testing.T, ep *Entrypoint, match []string, noMatch []string) {
	t.Helper()

	server, ok := ep.GetServer(":1000")
	require.True(t, ok, "server not found")
	require.NotNil(t, server)

	for _, test := range match {
		t.Run(test, func(t *testing.T) {
			route := server.FindRoute(test)
			assert.NotNil(t, route)
		})
	}

	for _, test := range noMatch {
		t.Run(test, func(t *testing.T) {
			found, ok := ep.HTTPRoutes().Get(test)
			assert.False(t, ok)
			assert.Nil(t, found)
		})
	}
}

func TestFindRouteAnyDomain(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	addRoute(t, "app1")

	tests := []string{
		"app1.com",
		"app1.domain.com",
		"app1.sub.domain.com",
	}
	testsNoMatch := []string{
		"sub.app1.com",
		"app2.com",
		"app2.domain.com",
		"app2.sub.domain.com",
	}

	run(t, ep, tests, testsNoMatch)
}

func TestFindRouteExactHostMatch(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	tests := []string{
		"app2.com",
		"app2.domain.com",
		"app2.sub.domain.com",
	}
	testsNoMatch := []string{
		"sub.app2.com",
		"app1.com",
		"app1.domain.com",
		"app1.sub.domain.com",
	}

	for _, test := range tests {
		addRoute(t, test)
	}

	run(t, ep, tests, testsNoMatch)
}

func TestFindRouteByDomains(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute(t, "app1")

	tests := []string{
		"app1.domain.com",
		"app1.sub.domain.com",
	}
	testsNoMatch := []string{
		"sub.app1.com",
		"app1.com",
		"app1.domain.co",
		"app1.domain.com.hk",
		"app1.sub.domain.co",
		"app2.domain.com",
		"app2.sub.domain.com",
	}

	run(t, ep, tests, testsNoMatch)
}

func TestFindRouteByDomainsExactMatch(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute(t, "app1.foo.bar")

	tests := []string{
		"app1.foo.bar", // exact match
		"app1.foo.bar.domain.com",
		"app1.foo.bar.sub.domain.com",
	}
	testsNoMatch := []string{
		"sub.app1.foo.bar",
		"sub.app1.foo.bar.com",
		"app1.domain.com",
		"app1.sub.domain.com",
	}

	run(t, ep, tests, testsNoMatch)
}

func TestFindRouteWithPort(t *testing.T) {
	t.Run("AnyDomain", func(t *testing.T) {
		ep := NewTestEntrypoint(t, nil)
		addRoute(t, "app1")
		addRoute(t, "app2.com")

		tests := []string{
			"app1:8080",
			"app1.domain.com:8080",
			"app2.com:8080",
		}
		testsNoMatch := []string{
			"app11",
			"app2.co",
			"app2.co:8080",
		}
		run(t, ep, tests, testsNoMatch)
	})

	t.Run("ByDomains", func(t *testing.T) {
		ep := NewTestEntrypoint(t, nil)
		ep.SetFindRouteDomains([]string{
			".domain.com",
		})
		addRoute(t, "app1")
		addRoute(t, "app2")
		addRoute(t, "app3.domain.com")

		tests := []string{
			"app1.domain.com:8080",
			"app2:8080", // exact match fallback
			"app3.domain.com:8080",
		}
		testsNoMatch := []string{
			"app11",
			"app1.domain.co",
			"app1.domain.co:8080",
			"app2.co",
			"app2.co:8080",
			"app3.domain.co",
			"app3.domain.co:8080",
		}
		run(t, ep, tests, testsNoMatch)
	})
}

func TestHealthInfoQueries(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	// Add routes without health monitors (default case)
	addRoute(t, "app1")
	addRoute(t, "app2")

	// Test GetHealthInfo
	t.Run("GetHealthInfo", func(t *testing.T) {
		info := ep.GetHealthInfo()
		expect.Equal(t, 2, len(info))
		for _, health := range info {
			expect.Equal(t, types.StatusUnknown, health.Status)
			expect.Equal(t, "n/a", health.Detail)
		}
	})

	// Test GetHealthInfoWithoutDetail
	t.Run("GetHealthInfoWithoutDetail", func(t *testing.T) {
		info := ep.GetHealthInfoWithoutDetail()
		expect.Equal(t, 2, len(info))
		for _, health := range info {
			expect.Equal(t, types.StatusUnknown, health.Status)
		}
	})

	// Test GetHealthInfoSimple
	t.Run("GetHealthInfoSimple", func(t *testing.T) {
		info := ep.GetHealthInfoSimple()
		expect.Equal(t, 2, len(info))
		for _, status := range info {
			expect.Equal(t, types.StatusUnknown, status)
		}
	})
}

func TestRoutesByProvider(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	// Add routes with provider info
	addRoute(t, "app1")
	addRoute(t, "app2")

	byProvider := ep.RoutesByProvider()
	expect.Equal(t, 1, len(byProvider)) // All routes are from same implicit provider

	routes, ok := byProvider[""]
	expect.True(t, ok)
	expect.Equal(t, 2, len(routes))
}

func TestNumRoutes(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	expect.Equal(t, 0, ep.NumRoutes())

	addRoute(t, "app1")
	expect.Equal(t, 1, ep.NumRoutes())

	addRoute(t, "app2")
	expect.Equal(t, 2, ep.NumRoutes())
}

func TestIterRoutes(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	addRoute(t, "app1")
	addRoute(t, "app2")
	addRoute(t, "app3")

	count := 0
	for r := range ep.IterRoutes {
		count++
		expect.NotNil(t, r)
	}
	expect.Equal(t, 3, count)
}

func TestGetRoute(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	// Route not found case
	_, ok := ep.GetRoute("nonexistent")
	expect.False(t, ok)

	addRoute(t, "app1")

	route, ok := ep.GetRoute("app1")
	expect.True(t, ok)
	expect.NotNil(t, route)
}

func TestHTTPRoutesPool(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	pool := ep.HTTPRoutes()
	expect.Equal(t, 0, pool.Size())

	addRoute(t, "app1")
	expect.Equal(t, 1, pool.Size())

	// Verify route is accessible
	route, ok := pool.Get("app1")
	expect.True(t, ok)
	expect.NotNil(t, route)
}

func TestExcludedRoutesPool(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	excludedPool := ep.ExcludedRoutes()
	expect.Equal(t, 0, excludedPool.Size())
}
