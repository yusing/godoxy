package routes

import (
	"time"

	"github.com/yusing/godoxy/internal/types"
)

type HealthInfo struct {
	HealthInfoWithoutDetail
	Detail string `json:"detail"`
} // @name HealthInfo

type HealthInfoWithoutDetail struct {
	Status  types.HealthStatus `json:"status" swaggertype:"string" enums:"healthy,unhealthy,napping,starting,error,unknown"`
	Uptime  time.Duration      `json:"uptime" swaggertype:"number"`  // uptime in milliseconds
	Latency time.Duration      `json:"latency" swaggertype:"number"` // latency in microseconds
} // @name HealthInfoWithoutDetail

func GetHealthInfo() map[string]HealthInfo {
	healthMap := make(map[string]HealthInfo, NumRoutes())
	for r := range Iter {
		healthMap[r.Name()] = getHealthInfo(r)
	}
	return healthMap
}

func GetHealthInfoWithoutDetail() map[string]HealthInfoWithoutDetail {
	healthMap := make(map[string]HealthInfoWithoutDetail, NumRoutes())
	for r := range Iter {
		healthMap[r.Name()] = getHealthInfoWithoutDetail(r)
	}
	return healthMap
}

func getHealthInfo(r types.Route) HealthInfo {
	mon := r.HealthMonitor()
	if mon == nil {
		return HealthInfo{
			HealthInfoWithoutDetail: HealthInfoWithoutDetail{
				Status: types.StatusUnknown,
			},
			Detail: "n/a",
		}
	}
	return HealthInfo{
		HealthInfoWithoutDetail: HealthInfoWithoutDetail{
			Status:  mon.Status(),
			Uptime:  mon.Uptime(),
			Latency: mon.Latency(),
		},
		Detail: mon.Detail(),
	}
}

func getHealthInfoWithoutDetail(r types.Route) HealthInfoWithoutDetail {
	mon := r.HealthMonitor()
	if mon == nil {
		return HealthInfoWithoutDetail{
			Status: types.StatusUnknown,
		}
	}
	return HealthInfoWithoutDetail{
		Status:  mon.Status(),
		Uptime:  mon.Uptime(),
		Latency: mon.Latency(),
	}
}

func ByProvider() map[string][]types.Route {
	rts := make(map[string][]types.Route)
	for r := range Iter {
		rts[r.ProviderName()] = append(rts[r.ProviderName()], r)
	}
	return rts
}
