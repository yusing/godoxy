package config

import (
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route/provider"
)

func (cfg *Config) VerifyNewAgent(host string, ca agent.PEMPair, client agent.PEMPair, containerRuntime agent.ContainerRuntime) (int, gperr.Error) {
	for _, a := range cfg.value.Providers.Agents {
		if a.Addr == host {
			return 0, gperr.New("agent already exists")
		}
	}

	agentCfg := agent.AgentConfig{
		Addr:    host,
		Runtime: containerRuntime,
	}
	err := agentCfg.StartWithCerts(cfg.Task().Context(), ca.Cert, client.Cert, client.Key)
	if err != nil {
		return 0, gperr.Wrap(err, "failed to start agent")
	}

	provider := provider.NewAgentProvider(&agentCfg)
	if _, loaded := cfg.providers.LoadOrStore(provider.String(), provider); loaded {
		return 0, gperr.Errorf("provider %s already exists", provider.String())
	}

	// agent must be added before loading routes
	agent.AddAgent(&agentCfg)
	err = provider.LoadRoutes()
	if err != nil {
		cfg.providers.Delete(provider.String())
		agent.RemoveAgent(&agentCfg)
		return 0, gperr.Wrap(err, "failed to load routes")
	}

	return provider.NumRoutes(), nil
}
