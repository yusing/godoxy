package routes

import (
	"time"

	"github.com/yusing/go-proxy/internal/homepage"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

func getHealthInfo(r Route) map[string]string {
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
	Status  health.Status `json:"status,string"`
	Latency time.Duration `json:"latency"`
}

func getHealthInfoRaw(r Route) *HealthInfoRaw {
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
	healthMap := make(map[string]map[string]string, NumRoutes())
	for alias, r := range Iter {
		healthMap[alias] = getHealthInfo(r)
	}
	return healthMap
}

func HealthInfo() map[string]*HealthInfoRaw {
	healthMap := make(map[string]*HealthInfoRaw, NumRoutes())
	for alias, r := range Iter {
		healthMap[alias] = getHealthInfoRaw(r)
	}
	return healthMap
}

func HomepageCategories() []string {
	check := make(map[string]struct{})
	categories := make([]string, 0)
	for _, r := range HTTP.Iter {
		item := r.HomepageConfig()
		if item == nil || item.Category == "" {
			continue
		}
		if _, ok := check[item.Category]; ok {
			continue
		}
		check[item.Category] = struct{}{}
		categories = append(categories, item.Category)
	}
	return categories
}

func HomepageConfig(categoryFilter, providerFilter string) homepage.Homepage {
	hp := make(homepage.Homepage)

	for _, r := range HTTP.Iter {
		if providerFilter != "" && r.ProviderName() != providerFilter {
			continue
		}
		item := r.HomepageItem()
		if categoryFilter != "" && item.Category != categoryFilter {
			continue
		}
		hp.Add(item)
	}
	return hp
}

func ByAlias(typeFilter ...route.RouteType) map[string]Route {
	rts := make(map[string]Route)
	if len(typeFilter) == 0 || typeFilter[0] == "" {
		typeFilter = []route.RouteType{route.RouteTypeHTTP, route.RouteTypeStream}
	}
	for _, t := range typeFilter {
		switch t {
		case route.RouteTypeHTTP:
			for alias, r := range HTTP.Iter {
				rts[alias] = r
			}
		case route.RouteTypeStream:
			for alias, r := range Stream.Iter {
				rts[alias] = r
			}
		}
	}
	return rts
}
