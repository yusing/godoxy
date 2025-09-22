package route

import (
	"github.com/yusing/godoxy/internal/gperr"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
)

func checkExists(r types.Route) gperr.Error {
	if r.UseLoadBalance() { // skip checking for load balanced routes
		return nil
	}
	var (
		existing types.Route
		ok       bool
	)
	switch r := r.(type) {
	case types.HTTPRoute:
		existing, ok = routes.HTTP.Get(r.Key())
	case types.StreamRoute:
		existing, ok = routes.Stream.Get(r.Key())
	}
	if ok {
		return gperr.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	return nil
}
