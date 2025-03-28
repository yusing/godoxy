package routequery

import (
	"time"

	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/route/routes"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

func getHealthInfo(r route.Route) map[string]string {
	mon := r.HealthMonitor()
	if mon == nil {
		return map[string]string{
			"status":  "unknown",
			"uptime":  "n/a",
			"latency": "n/a",
		}
	}
	return map[string]string{
		"status":  mon.Status().String(),
		"uptime":  mon.Uptime().Round(time.Second).String(),
		"latency": mon.Latency().Round(time.Microsecond).String(),
	}
}

type HealthInfoRaw struct {
	Status  health.Status
	Latency time.Duration
}

func getHealthInfoRaw(r route.Route) *HealthInfoRaw {
	mon := r.HealthMonitor()
	if mon == nil {
		return &HealthInfoRaw{
			Status:  health.StatusUnknown,
			Latency: time.Duration(0),
		}
	}
	return &HealthInfoRaw{
		Status:  mon.Status(),
		Latency: mon.Latency(),
	}
}

func HealthMap() map[string]map[string]string {
	healthMap := make(map[string]map[string]string, routes.NumRoutes())
	routes.RangeRoutes(func(alias string, r route.Route) {
		healthMap[alias] = getHealthInfo(r)
	})
	return healthMap
}

func HealthInfo() map[string]*HealthInfoRaw {
	healthMap := make(map[string]*HealthInfoRaw, routes.NumRoutes())
	routes.RangeRoutes(func(alias string, r route.Route) {
		healthMap[alias] = getHealthInfoRaw(r)
	})
	return healthMap
}

func HomepageCategories() []string {
	check := make(map[string]struct{})
	categories := make([]string, 0)
	routes.GetHTTPRoutes().RangeAll(func(alias string, r route.HTTPRoute) {
		item := r.HomepageConfig()
		if item == nil || item.Category == "" {
			return
		}
		if _, ok := check[item.Category]; ok {
			return
		}
		check[item.Category] = struct{}{}
		categories = append(categories, item.Category)
	})
	return categories
}

func HomepageConfig(categoryFilter, providerFilter string) homepage.Homepage {
	hp := make(homepage.Homepage)

	routes.GetHTTPRoutes().RangeAll(func(alias string, r route.HTTPRoute) {
		if providerFilter != "" && r.ProviderName() != providerFilter {
			return
		}
		item := r.HomepageItem()
		if categoryFilter != "" && item.Category != categoryFilter {
			return
		}
		hp.Add(item)
	})
	return hp
}

func RoutesByAlias(typeFilter ...route.RouteType) map[string]route.Route {
	rts := make(map[string]route.Route)
	if len(typeFilter) == 0 || typeFilter[0] == "" {
		typeFilter = []route.RouteType{route.RouteTypeHTTP, route.RouteTypeStream}
	}
	for _, t := range typeFilter {
		switch t {
		case route.RouteTypeHTTP:
			routes.GetHTTPRoutes().RangeAll(func(alias string, r route.HTTPRoute) {
				rts[alias] = r
			})
		case route.RouteTypeStream:
			routes.GetStreamRoutes().RangeAll(func(alias string, r route.StreamRoute) {
				rts[alias] = r
			})
		}
	}
	return rts
}
