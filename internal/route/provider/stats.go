package provider

import (
	R "github.com/yusing/go-proxy/internal/route"
	provider "github.com/yusing/go-proxy/internal/route/provider/types"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type (
	RouteStats struct {
		Total        uint16 `json:"total"`
		NumHealthy   uint16 `json:"healthy"`
		NumUnhealthy uint16 `json:"unhealthy"`
		NumNapping   uint16 `json:"napping"`
		NumError     uint16 `json:"error"`
		NumUnknown   uint16 `json:"unknown"`
	}
	ProviderStats struct {
		Total   uint16        `json:"total"`
		RPs     RouteStats    `json:"reverse_proxies"`
		Streams RouteStats    `json:"streams"`
		Type    provider.Type `json:"type"`
	}
)

func (stats *RouteStats) Add(r *R.Route) {
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

func (p *Provider) Statistics() ProviderStats {
	var rps, streams RouteStats
	for _, r := range p.routes {
		switch r.Type() {
		case route.RouteTypeHTTP:
			rps.Add(r)
		case route.RouteTypeStream:
			streams.Add(r)
		}
	}
	return ProviderStats{
		Total:   rps.Total + streams.Total,
		RPs:     rps,
		Streams: streams,
		Type:    p.t,
	}
}
