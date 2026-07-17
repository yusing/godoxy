package entrypoint

import (
	"time"

	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/routing"
)

// GetHealthInfo returns a map of route name to health info.
//
// The health info is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfo() map[string]health.HealthInfo {
	healthMap := make(map[string]health.HealthInfo, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfo(r)
	}
	return healthMap
}

// GetHealthInfoWithoutDetail returns a map of route name to health info without detail.
//
// The health info is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfoWithoutDetail() map[string]health.HealthInfoWithoutDetail {
	healthMap := make(map[string]health.HealthInfoWithoutDetail, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfoWithoutDetail(r)
	}
	return healthMap
}

// GetHealthInfoSimple returns a map of route name to health status.
//
// The health status is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfoSimple() map[string]health.HealthStatus {
	healthMap := make(map[string]health.HealthStatus, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfoSimple(r)
	}
	return healthMap
}

// RoutesByProvider returns a map of provider name to routes.
//
// The routes are all routes, including excluded routes.
func (ep *Entrypoint) RoutesByProvider() map[string][]routing.Route {
	rts := make(map[string][]routing.Route)
	for r := range ep.IterRoutes {
		providerName := r.ProviderName()
		rts[providerName] = append(rts[providerName], r)
	}
	return rts
}

func getHealthInfo(r routing.Route) health.HealthInfo {
	mon := r.HealthMonitor()
	if mon == nil {
		return health.HealthInfo{
			HealthInfoWithoutDetail: health.HealthInfoWithoutDetail{
				Status: health.StatusUnknown,
			},
			Detail: "n/a",
		}
	}
	return health.HealthInfo{
		HealthInfoWithoutDetail: health.HealthInfoWithoutDetail{
			Status:  mon.Status(),
			Uptime:  mon.Uptime(),
			Latency: mon.Latency(),
			SleepIn: sleepIn(mon),
		},
		Detail: mon.Detail(),
	}
}

func getHealthInfoWithoutDetail(r routing.Route) health.HealthInfoWithoutDetail {
	mon := r.HealthMonitor()
	if mon == nil {
		return health.HealthInfoWithoutDetail{
			Status: health.StatusUnknown,
		}
	}
	return health.HealthInfoWithoutDetail{
		Status:  mon.Status(),
		Uptime:  mon.Uptime(),
		Latency: mon.Latency(),
		SleepIn: sleepIn(mon),
	}
}

func sleepIn(mon health.HealthMonitor) time.Duration {
	timer, ok := mon.(health.SleepTimer)
	if !ok {
		return 0
	}
	return timer.SleepIn()
}

func getHealthInfoSimple(r routing.Route) health.HealthStatus {
	mon := r.HealthMonitor()
	if mon == nil {
		return health.StatusUnknown
	}
	return mon.Status()
}
