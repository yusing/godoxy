package entrypoint

import (
	"testing"

	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/route/routes"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

var ep = NewEntrypoint()

func addRoute(alias string) *route.ReveseProxyRoute {
	r := &route.ReveseProxyRoute{
		Route: &route.Route{
			Alias: alias,
		},
	}
	routes.HTTP.Add(r)
	return r
}

func run(t *testing.T, match []string, noMatch []string) {
	t.Helper()
	t.Cleanup(routes.Clear)
	t.Cleanup(func() { ep.SetFindRouteDomains(nil) })

	for _, test := range match {
		t.Run(test, func(t *testing.T) {
			r := addRoute(test)
			found, err := ep.findRouteFunc(test)
			expect.NoError(t, err)
			expect.True(t, found == r)
		})
	}

	for _, test := range noMatch {
		t.Run(test, func(t *testing.T) {
			_, err := ep.findRouteFunc(test)
			expect.ErrorIs(t, ErrNoSuchRoute, err)
		})
	}
}

func TestFindRouteAnyDomain(t *testing.T) {
	addRoute("app1")

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
		addRoute(test)
	}

	run(t, tests, testsNoMatch)
}

func TestFindRouteByDomains(t *testing.T) {
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute("app1")

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
	ep.SetFindRouteDomains([]string{
		".domain.com",
		".sub.domain.com",
	})

	addRoute("app1")

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
