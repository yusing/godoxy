package entrypoint_test

import (
	"testing"

	. "github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/route"

	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func addRoute(ep *Entrypoint, alias string) {
	ep.AddRoute(&route.ReveseProxyRoute{
		Route: &route.Route{
			Alias: alias,
			Port: route.Port{
				Proxy: 80,
			},
		},
	})
}

func run(t *testing.T, match []string, noMatch []string) {
	t.Helper()
	ep := NewEntrypoint(task.NewTestTask(t), nil)

	for _, test := range match {
		t.Run(test, func(t *testing.T) {
			found, ok := ep.HTTPRoutes().Get(test)
			expect.True(t, ok)
			expect.NotNil(t, found)
		})
	}

	for _, test := range noMatch {
		t.Run(test, func(t *testing.T) {
			found, ok := ep.HTTPRoutes().Get(test)
			expect.False(t, ok)
			expect.Nil(t, found)
		})
	}
}

func TestFindRouteAnyDomain(t *testing.T) {
	ep := NewEntrypoint(task.NewTestTask(t), nil)
	addRoute(ep, "app1")

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

	run(t, tests, testsNoMatch)
}

func TestFindRouteExactHostMatch(t *testing.T) {
	ep := NewEntrypoint(task.NewTestTask(t), nil)
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
		addRoute(ep, test)
	}

	run(t, tests, testsNoMatch)
}

func TestFindRouteByDomains(t *testing.T) {
	ep := NewEntrypoint(task.NewTestTask(t), nil)
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute(ep, "app1")

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

	run(t, tests, testsNoMatch)
}

func TestFindRouteByDomainsExactMatch(t *testing.T) {
	ep := NewEntrypoint(task.NewTestTask(t), nil)
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute(ep, "app1.foo.bar")

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

	run(t, tests, testsNoMatch)
}

func TestFindRouteWithPort(t *testing.T) {
	t.Run("AnyDomain", func(t *testing.T) {
		ep := NewEntrypoint(task.NewTestTask(t), nil)
		addRoute(ep, "app1")
		addRoute(ep, "app2.com")

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
		run(t, tests, testsNoMatch)
	})

	t.Run("ByDomains", func(t *testing.T) {
		ep := NewEntrypoint(task.NewTestTask(t), nil)
		ep.SetFindRouteDomains([]string{
			".domain.com",
		})
		addRoute(ep, "app1")
		addRoute(ep, "app2")
		addRoute(ep, "app3.domain.com")

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
		run(t, tests, testsNoMatch)
	})
}
