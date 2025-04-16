package agent

import (
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type agents struct{ pool.Pool[*AgentConfig] }

var Agents = agents{pool.New[*AgentConfig]("agents")}

func (agents agents) Get(agentAddrOrDockerHost string) (*AgentConfig, bool) {
	if !IsDockerHostAgent(agentAddrOrDockerHost) {
		return agents.Base().Load(agentAddrOrDockerHost)
	}
	return agents.Base().Load(GetAgentAddrFromDockerHost(agentAddrOrDockerHost))
}
