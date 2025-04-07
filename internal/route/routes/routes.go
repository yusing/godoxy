package routes

import (
	route "github.com/yusing/go-proxy/internal/route/types"
	F "github.com/yusing/go-proxy/internal/utils/functional"
)

var (
	httpRoutes   = F.NewMapOf[string, route.HTTPRoute]()
	streamRoutes = F.NewMapOf[string, route.StreamRoute]()
)

func RangeRoutes(callback func(alias string, r route.Route)) {
	httpRoutes.RangeAll(func(alias string, r route.HTTPRoute) {
		callback(alias, r)
	})
	streamRoutes.RangeAll(func(alias string, r route.StreamRoute) {
		callback(alias, r)
	})
}

func NumRoutes() int {
	return httpRoutes.Size() + streamRoutes.Size()
}

func GetHTTPRoutes() F.Map[string, route.HTTPRoute] {
	return httpRoutes
}

func GetStreamRoutes() F.Map[string, route.StreamRoute] {
	return streamRoutes
}

func GetHTTPRouteOrExact(alias, host string) (route.HTTPRoute, bool) {
	r, ok := httpRoutes.Load(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return httpRoutes.Load(host)
}

func GetHTTPRoute(alias string) (route.HTTPRoute, bool) {
	return httpRoutes.Load(alias)
}

func GetStreamRoute(alias string) (route.StreamRoute, bool) {
	return streamRoutes.Load(alias)
}

func GetRoute(alias string) (route.Route, bool) {
	r, ok := httpRoutes.Load(alias)
	if ok {
		return r, true
	}
	return streamRoutes.Load(alias)
}

func SetHTTPRoute(alias string, r route.HTTPRoute) {
	httpRoutes.Store(alias, r)
}

func SetStreamRoute(alias string, r route.StreamRoute) {
	streamRoutes.Store(alias, r)
}

func DeleteHTTPRoute(alias string) {
	httpRoutes.Delete(alias)
}

func DeleteStreamRoute(alias string) {
	streamRoutes.Delete(alias)
}

func TestClear() {
	httpRoutes = F.NewMapOf[string, route.HTTPRoute]()
	streamRoutes = F.NewMapOf[string, route.StreamRoute]()
}
