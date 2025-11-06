package monitor

import (
	"net/url"

	agentPkg "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/synk"
)

type (
	AgentProxiedMonitor struct {
		agent *agentPkg.AgentConfig
		query synk.Value[string]
		*monitor
	}
	AgentCheckHealthTarget struct {
		Scheme string
		Host   string
		Path   string
	}
)

func AgentTargetFromURL(url *url.URL) *AgentCheckHealthTarget {
	return &AgentCheckHealthTarget{
		Scheme: url.Scheme,
		Host:   url.Host,
		Path:   url.Path,
	}
}

func (target *AgentCheckHealthTarget) buildQuery() string {
	query := make(url.Values, 3)
	query.Set("scheme", target.Scheme)
	query.Set("host", target.Host)
	query.Set("path", target.Path)
	return query.Encode()
}

func (target *AgentCheckHealthTarget) displayURL() *url.URL {
	return &url.URL{
		Scheme: target.Scheme,
		Host:   target.Host,
		Path:   target.Path,
	}
}

func NewAgentProxiedMonitor(agent *agentPkg.AgentConfig, config *types.HealthCheckConfig, target *AgentCheckHealthTarget) *AgentProxiedMonitor {
	mon := &AgentProxiedMonitor{
		agent: agent,
	}
	mon.monitor = newMonitor(target.displayURL(), config, mon.CheckHealth)
	mon.query.Store(target.buildQuery())
	return mon
}

func (mon *AgentProxiedMonitor) CheckHealth() (types.HealthCheckResult, error) {
	resp, err := mon.agent.DoHealthCheck(mon.config.Timeout, mon.query.Load())
	result := types.HealthCheckResult{
		Healthy: resp.Healthy,
		Detail:  resp.Detail,
		Latency: resp.Latency,
	}
	return result, err
}

func (mon *AgentProxiedMonitor) UpdateURL(url *url.URL) {
	mon.monitor.UpdateURL(url)
	newTarget := AgentTargetFromURL(url)
	mon.query.Store(newTarget.buildQuery())
}
