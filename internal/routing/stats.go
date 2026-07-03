package routing

import (
	"github.com/yusing/godoxy/internal/health"
)

type (
	RouteStats struct {
		Total        uint16 `json:"total"`
		NumHealthy   uint16 `json:"healthy"`
		NumUnhealthy uint16 `json:"unhealthy"`
		NumNapping   uint16 `json:"napping"`
		NumError     uint16 `json:"error"`
		NumUnknown   uint16 `json:"unknown"`
	} // @name RouteStats
	ProviderStats struct {
		Total   uint16       `json:"total"`
		RPs     RouteStats   `json:"reverse_proxies"`
		Streams RouteStats   `json:"streams"`
		Type    ProviderType `json:"type"`
	} // @name ProviderStats
)

func (stats *RouteStats) Add(r Route) {
	stats.Total++
	mon := r.HealthMonitor()
	if mon == nil {
		stats.NumUnknown++
		return
	}
	switch mon.Status() {
	case health.StatusHealthy:
		stats.NumHealthy++
	case health.StatusUnhealthy:
		stats.NumUnhealthy++
	case health.StatusNapping:
		stats.NumNapping++
	case health.StatusError:
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
