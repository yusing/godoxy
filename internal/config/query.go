package config

import (
	"slices"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
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

func (cfg *Config) RouteProviderList() []string {
	var list []string
	cfg.providers.RangeAll(func(_ string, p *provider.Provider) {
		list = append(list, p.ShortName())
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

func (cfg *Config) VerifyNewAgent(host string, ca agent.PEMPair, client agent.PEMPair) (int, gperr.Error) {
	if slices.ContainsFunc(cfg.value.Providers.Agents, func(a *agent.AgentConfig) bool {
		return a.Addr == host
	}) {
		return 0, gperr.New("agent already exists")
	}

	agentCfg := new(agent.AgentConfig)
	agentCfg.Addr = host
	err := agentCfg.InitWithCerts(cfg.task.Context(), ca.Cert, client.Cert, client.Key)
	if err != nil {
		return 0, gperr.Wrap(err, "failed to start agent")
	}
	// must add it first to let LoadRoutes() reference from it
	agent.Agents.Add(agentCfg)

	provider := provider.NewAgentProvider(agentCfg)
	if err := cfg.errIfExists(provider); err != nil {
		agent.Agents.Del(agentCfg)
		return 0, err
	}
	err = provider.LoadRoutes()
	if err != nil {
		agent.Agents.Del(agentCfg)
		return 0, gperr.Wrap(err, "failed to load routes")
	}
	return provider.NumRoutes(), nil
}
