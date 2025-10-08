package statequery

import (
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/types"
)

type Statistics struct {
	Total          uint16                         `json:"total"`
	ReverseProxies types.RouteStats               `json:"reverse_proxies"`
	Streams        types.RouteStats               `json:"streams"`
	Providers      map[string]types.ProviderStats `json:"providers"`
}

func GetStatistics() Statistics {
	state := config.ActiveState.Load()

	var (
		rps, streams  types.RouteStats
		total         uint16
		providerStats = make(map[string]types.ProviderStats)
	)

	for _, p := range state.IterProviders() {
		stats := p.Statistics()
		providerStats[p.ShortName()] = stats
		rps.AddOther(stats.RPs)
		streams.AddOther(stats.Streams)
		total += stats.RPs.Total + stats.Streams.Total
	}

	return Statistics{
		Total:          total,
		ReverseProxies: rps,
		Streams:        streams,
		Providers:      providerStats,
	}
}
