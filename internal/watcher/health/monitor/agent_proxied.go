package monitor

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	agentPkg "github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/watcher/health"
)

type (
	AgentProxiedMonitor struct {
		agent       *agentPkg.AgentConfig
		endpointURL string
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

func NewAgentProxiedMonitor(agent *agentPkg.AgentConfig, config *health.HealthCheckConfig, target *AgentCheckHealthTarget) *AgentProxiedMonitor {
	mon := &AgentProxiedMonitor{
		agent:       agent,
		endpointURL: agentPkg.EndpointHealth + "?" + target.buildQuery(),
	}
	mon.monitor = newMonitor(target.displayURL(), config, mon.CheckHealth)
	return mon
}

func (mon *AgentProxiedMonitor) CheckHealth() (result *health.HealthCheckResult, err error) {
	startTime := time.Now()
	result = new(health.HealthCheckResult)
	ctx, cancel := mon.ContextWithTimeout("timeout querying agent")
	defer cancel()
	data, status, err := mon.agent.Fetch(ctx, mon.endpointURL)
	if err != nil {
		return result, err
	}
	endTime := time.Now()
	switch status {
	case http.StatusOK:
		err = json.Unmarshal(data, result)
	default:
		err = errors.New(string(data))
	}
	if err == nil && result.Latency != 0 {
		// use godoxy to agent latency
		result.Latency = endTime.Sub(startTime)
	}
	return
}
