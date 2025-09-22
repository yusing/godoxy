package types

import provider "github.com/yusing/godoxy/internal/route/provider/types"

type (
	RouteStats struct {
		Total        uint16 `json:"total"`
		NumHealthy   uint16 `json:"healthy"`
		NumUnhealthy uint16 `json:"unhealthy"`
		NumNapping   uint16 `json:"napping"`
		NumError     uint16 `json:"error"`
		NumUnknown   uint16 `json:"unknown"`
	} //	@name	RouteStats
	ProviderStats struct {
		Total   uint16        `json:"total"`
		RPs     RouteStats    `json:"reverse_proxies"`
		Streams RouteStats    `json:"streams"`
		Type    provider.Type `json:"type"`
	} //	@name	ProviderStats
)

func (stats *RouteStats) Add(r Route) {
	stats.Total++
	mon := r.HealthMonitor()
	if mon == nil {
		stats.NumUnknown++
		return
	}
	switch mon.Status() {
	case StatusHealthy:
		stats.NumHealthy++
	case StatusUnhealthy:
		stats.NumUnhealthy++
	case StatusNapping:
		stats.NumNapping++
	case StatusError:
		stats.NumError++
	default:
		stats.NumUnknown++
	}
}

func (stats *RouteStats) AddOther(other RouteStats) {
	stats.Total += other.Total
	stats.NumHealthy += other.NumHealthy
	stats.NumUnhealthy += other.NumUnhealthy
	stats.NumNapping += other.NumNapping
	stats.NumError += other.NumError
	stats.NumUnknown += other.NumUnknown
}
