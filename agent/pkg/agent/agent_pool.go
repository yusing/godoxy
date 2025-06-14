package agent

import (
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/utils/functional"
)

var agentPool = functional.NewMapOf[string, *AgentConfig]()

func init() {
	if common.IsTest {
		agentPool.Store("test-agent", &AgentConfig{
			Addr: "test-agent",
		})
	}
}

func GetAgent(agentAddrOrDockerHost string) (*AgentConfig, bool) {
	if !IsDockerHostAgent(agentAddrOrDockerHost) {
		return getAgentByAddr(agentAddrOrDockerHost)
	}
	return getAgentByAddr(GetAgentAddrFromDockerHost(agentAddrOrDockerHost))
}

func GetAgentByName(name string) (*AgentConfig, bool) {
	for _, agent := range agentPool.Range {
		if agent.Name() == name {
			return agent, true
		}
	}
	return nil, false
}

func AddAgent(agent *AgentConfig) {
	agentPool.Store(agent.Addr, agent)
}

func RemoveAgent(agent *AgentConfig) {
	agentPool.Delete(agent.Addr)
}

func RemoveAllAgents() {
	agentPool.Clear()
}

func ListAgents() []*AgentConfig {
	agents := make([]*AgentConfig, 0, agentPool.Size())
	for _, agent := range agentPool.Range {
		agents = append(agents, agent)
	}
	return agents
}

func getAgentByAddr(addr string) (agent *AgentConfig, ok bool) {
	agent, ok = agentPool.Load(addr)
	return
}
