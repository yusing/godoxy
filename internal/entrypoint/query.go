package entrypoint

import (
	"github.com/yusing/godoxy/internal/types"
)

// GetHealthInfo returns a map of route name to health info.
//
// The health info is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfo() map[string]types.HealthInfo {
	healthMap := make(map[string]types.HealthInfo, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfo(r)
	}
	return healthMap
}

// GetHealthInfoWithoutDetail returns a map of route name to health info without detail.
//
// The health info is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfoWithoutDetail() map[string]types.HealthInfoWithoutDetail {
	healthMap := make(map[string]types.HealthInfoWithoutDetail, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfoWithoutDetail(r)
	}
	return healthMap
}

// GetHealthInfoSimple returns a map of route name to health status.
//
// The health status is for all routes, including excluded routes.
func (ep *Entrypoint) GetHealthInfoSimple() map[string]types.HealthStatus {
	healthMap := make(map[string]types.HealthStatus, ep.NumRoutes())
	for r := range ep.IterRoutes {
		healthMap[r.Name()] = getHealthInfoSimple(r)
	}
	return healthMap
}

// RoutesByProvider returns a map of provider name to routes.
//
// The routes are all routes, including excluded routes.
func (ep *Entrypoint) RoutesByProvider() map[string][]types.Route {
	rts := make(map[string][]types.Route)
	for r := range ep.IterRoutes {
		rts[r.ProviderName()] = append(rts[r.ProviderName()], r)
	}
	return rts
}

func getHealthInfo(r types.Route) types.HealthInfo {
	mon := r.HealthMonitor()
	if mon == nil {
		return types.HealthInfo{
			HealthInfoWithoutDetail: types.HealthInfoWithoutDetail{
				Status: types.StatusUnknown,
			},
			Detail: "n/a",
		}
	}
	return types.HealthInfo{
		HealthInfoWithoutDetail: types.HealthInfoWithoutDetail{
			Status:  mon.Status(),
			Uptime:  mon.Uptime(),
			Latency: mon.Latency(),
		},
		Detail: mon.Detail(),
	}
}

func getHealthInfoWithoutDetail(r types.Route) types.HealthInfoWithoutDetail {
	mon := r.HealthMonitor()
	if mon == nil {
		return types.HealthInfoWithoutDetail{
			Status: types.StatusUnknown,
		}
	}
	return types.HealthInfoWithoutDetail{
		Status:  mon.Status(),
		Uptime:  mon.Uptime(),
		Latency: mon.Latency(),
	}
}

func getHealthInfoSimple(r types.Route) types.HealthStatus {
	mon := r.HealthMonitor()
	if mon == nil {
		return types.StatusUnknown
	}
	return mon.Status()
}
