package agent

import (
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type agents struct{ pool.Pool[*AgentConfig] }

var Agents = agents{pool.New[*AgentConfig]("agents")}

func (agents agents) Get(agentAddrOrDockerHost string) (*AgentConfig, bool) {
	if !IsDockerHostAgent(agentAddrOrDockerHost) {
		return agents.Get(agentAddrOrDockerHost)
	}
	return agents.Get(GetAgentAddrFromDockerHost(agentAddrOrDockerHost))
}
