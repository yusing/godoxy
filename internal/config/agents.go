package config

import (
	"slices"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route/provider"
)

func (cfg *Config) VerifyNewAgent(host string, ca agent.PEMPair, client agent.PEMPair) (int, gperr.Error) {
	if slices.ContainsFunc(cfg.value.Providers.Agents, func(a *agent.AgentConfig) bool {
		return a.Addr == host
	}) {
		return 0, gperr.New("agent already exists")
	}

	var agentCfg agent.AgentConfig
	agentCfg.Addr = host
	err := agentCfg.StartWithCerts(cfg.Task().Context(), ca.Cert, client.Cert, client.Key)
	if err != nil {
		return 0, gperr.Wrap(err, "failed to start agent")
	}
	agent.AddAgent(&agentCfg)

	provider := provider.NewAgentProvider(&agentCfg)
	if err := cfg.errIfExists(provider); err != nil {
		agent.RemoveAgent(&agentCfg)
		return 0, err
	}
	err = provider.LoadRoutes()
	if err != nil {
		agent.RemoveAgent(&agentCfg)
		return 0, gperr.Wrap(err, "failed to load routes")
	}
	return provider.NumRoutes(), nil
}
