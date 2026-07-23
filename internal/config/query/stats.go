package statequery

import (
	"context"

	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/routing"
)

type Statistics struct {
	Total          uint16                           `json:"total"`
	ReverseProxies routing.RouteStats               `json:"reverse_proxies"`
	Streams        routing.RouteStats               `json:"streams"`
	Providers      map[string]routing.ProviderStats `json:"providers"`
}

func GetStatistics(ctx context.Context) Statistics {
	state := config.FromCtx(ctx)

	var (
		rps, streams  routing.RouteStats
		total         uint16
		providerStats = make(map[string]routing.ProviderStats)
	)
	if state == nil {
		return Statistics{Providers: providerStats}
	}

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
