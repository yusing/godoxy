package monitor

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/bytedance/sonic"
	agentPkg "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/types"
	httputils "github.com/yusing/goutils/http"
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

func NewAgentProxiedMonitor(agent *agentPkg.AgentConfig, config *types.HealthCheckConfig, target *AgentCheckHealthTarget) *AgentProxiedMonitor {
	mon := &AgentProxiedMonitor{
		agent:       agent,
		endpointURL: agentPkg.EndpointHealth + "?" + target.buildQuery(),
	}
	mon.monitor = newMonitor(target.displayURL(), config, mon.CheckHealth)
	return mon
}

func (mon *AgentProxiedMonitor) CheckHealth() (result types.HealthCheckResult, err error) {
	startTime := time.Now()

	ctx, cancel := mon.ContextWithTimeout("timeout querying agent")
	defer cancel()
	resp, err := mon.agent.DoHealthCheck(ctx, mon.endpointURL)
	if err != nil {
		return result, err
	}

	data, release, err := httputils.ReadAllBody(resp)
	resp.Body.Close()
	if err != nil {
		return result, err
	}
	defer release(data)

	endTime := time.Now()
	switch resp.StatusCode {
	case http.StatusOK:
		err = sonic.Unmarshal(data, &result)
	default:
		err = fmt.Errorf("HTTP %d %s", resp.StatusCode, data)
	}
	if err == nil && result.Latency != 0 {
		// use godoxy to agent latency
		result.Latency = endTime.Sub(startTime)
	}
	return result, err
}
