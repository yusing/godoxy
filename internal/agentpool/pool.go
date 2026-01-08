package agentpool

import (
	"iter"
	"os"
	"strings"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/godoxy/agent/pkg/agent"
)

var agentPool = xsync.NewMap[string, *Agent](xsync.WithPresize(10))

func init() {
	if strings.HasSuffix(os.Args[0], ".test") {
		agentPool.Store("test-agent", &Agent{
			AgentConfig: &agent.AgentConfig{
				Addr: "test-agent",
			},
		})
	}
}

func Get(agentAddrOrDockerHost string) (*Agent, bool) {
	if !agent.IsDockerHostAgent(agentAddrOrDockerHost) {
		return getAgentByAddr(agentAddrOrDockerHost)
	}
	return getAgentByAddr(agent.GetAgentAddrFromDockerHost(agentAddrOrDockerHost))
}

func GetAgent(name string) (*Agent, bool) {
	for _, agent := range agentPool.Range {
		if agent.Name == name {
			return agent, true
		}
	}
	return nil, false
}

func Add(cfg *agent.AgentConfig) (added bool) {
	_, loaded := agentPool.LoadOrCompute(cfg.Addr, func() (*Agent, bool) {
		return newAgent(cfg), false
	})
	return !loaded
}

func Has(cfg *agent.AgentConfig) bool {
	_, ok := agentPool.Load(cfg.Addr)
	return ok
}

func Remove(cfg *agent.AgentConfig) {
	agentPool.Delete(cfg.Addr)
}

func RemoveAll() {
	agentPool.Clear()
}

func List() []*Agent {
	agents := make([]*Agent, 0, agentPool.Size())
	for _, agent := range agentPool.Range {
		agents = append(agents, agent)
	}
	return agents
}

func Iter() iter.Seq2[string, *Agent] {
	return agentPool.Range
}

func Num() int {
	return agentPool.Size()
}

func getAgentByAddr(addr string) (agent *Agent, ok bool) {
	agent, ok = agentPool.Load(addr)
	return agent, ok
}
