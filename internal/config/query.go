package config

import (
	config "github.com/yusing/go-proxy/internal/config/types"
	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/route/provider"
)

func (cfg *Config) DumpRoutes() map[string]*route.Route {
	entries := make(map[string]*route.Route)
	cfg.providers.RangeAll(func(_ string, p *provider.Provider) {
		p.RangeRoutes(func(alias string, r *route.Route) {
			entries[alias] = r
		})
	})
	return entries
}

func (cfg *Config) DumpRouteProviders() map[string]*provider.Provider {
	entries := make(map[string]*provider.Provider)
	cfg.providers.RangeAll(func(_ string, p *provider.Provider) {
		entries[p.ShortName()] = p
	})
	return entries
}

func (cfg *Config) RouteProviderList() []config.RouteProviderListResponse {
	var list []config.RouteProviderListResponse
	cfg.providers.RangeAll(func(_ string, p *provider.Provider) {
		list = append(list, config.RouteProviderListResponse{
			ShortName: p.ShortName(),
			FullName:  p.String(),
		})
	})
	return list
}

func (cfg *Config) Statistics() map[string]any {
	var rps, streams provider.RouteStats
	var total uint16
	providerStats := make(map[string]provider.ProviderStats)

	cfg.providers.RangeAll(func(_ string, p *provider.Provider) {
		stats := p.Statistics()
		providerStats[p.ShortName()] = stats
		rps.AddOther(stats.RPs)
		streams.AddOther(stats.Streams)
		total += stats.RPs.Total + stats.Streams.Total
	})

	return map[string]any{
		"total":           total,
		"reverse_proxies": rps,
		"streams":         streams,
		"providers":       providerStats,
	}
}
