package route

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route/routes"
)

func checkExists(r routes.Route) gperr.Error {
	if r.UseLoadBalance() { // skip checking for load balanced routes
		return nil
	}
	var (
		existing routes.Route
		ok       bool
	)
	switch r := r.(type) {
	case routes.HTTPRoute:
		existing, ok = routes.HTTP.Get(r.Key())
	case routes.StreamRoute:
		existing, ok = routes.Stream.Get(r.Key())
	}
	if ok {
		return gperr.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	return nil
}
