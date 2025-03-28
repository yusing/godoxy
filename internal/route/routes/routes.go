package routes

import (
	"github.com/yusing/go-proxy/internal/route/types"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

var (
	httpRoutes   = F.NewMapOf[string, types.HTTPRoute]()
	streamRoutes = F.NewMapOf[string, types.StreamRoute]()
)

func RangeRoutes(callback func(alias string, r types.Route)) {
	httpRoutes.RangeAll(func(alias string, r types.HTTPRoute) {
		callback(alias, r)
	})
	streamRoutes.RangeAll(func(alias string, r types.StreamRoute) {
		callback(alias, r)
	})
}

func NumRoutes() int {
	return httpRoutes.Size() + streamRoutes.Size()
}

func GetHTTPRoutes() F.Map[string, types.HTTPRoute] {
	return httpRoutes
}

func GetStreamRoutes() F.Map[string, types.StreamRoute] {
	return streamRoutes
}

func GetHTTPRouteOrExact(alias, host string) (types.HTTPRoute, bool) {
	r, ok := httpRoutes.Load(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return httpRoutes.Load(host)
}

func GetHTTPRoute(alias string) (types.HTTPRoute, bool) {
	return httpRoutes.Load(alias)
}

func GetStreamRoute(alias string) (types.StreamRoute, bool) {
	return streamRoutes.Load(alias)
}

func GetRoute(alias string) (types.Route, bool) {
	r, ok := httpRoutes.Load(alias)
	if ok {
		return r, true
	}
	return streamRoutes.Load(alias)
}

func SetHTTPRoute(alias string, r types.HTTPRoute) {
	httpRoutes.Store(alias, r)
}

func SetStreamRoute(alias string, r types.StreamRoute) {
	streamRoutes.Store(alias, r)
}

func DeleteHTTPRoute(alias string) {
	httpRoutes.Delete(alias)
}

func DeleteStreamRoute(alias string) {
	streamRoutes.Delete(alias)
}

func TestClear() {
	httpRoutes = F.NewMapOf[string, types.HTTPRoute]()
	streamRoutes = F.NewMapOf[string, types.StreamRoute]()
}
