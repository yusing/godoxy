package agentpool

import (
	"context"
	"iter"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/godoxy/agent/pkg/agent"
)

type Pool struct {
	agents *xsync.Map[string, *Agent]
}

func NewPool() *Pool {
	return &Pool{agents: xsync.NewMap[string, *Agent](xsync.WithPresize(10))}
}

func (pool *Pool) Get(agentAddrOrDockerHost string) (*Agent, bool) {
	if !agent.IsDockerHostAgent(agentAddrOrDockerHost) {
		return pool.getAgentByAddr(agentAddrOrDockerHost)
	}
	return pool.getAgentByAddr(agent.GetAgentAddrFromDockerHost(agentAddrOrDockerHost))
}

func (pool *Pool) GetAgent(name string) (*Agent, bool) {
	for _, agent := range pool.agents.Range {
		if agent.Name == name {
			return agent, true
		}
	}
	return nil, false
}

func (pool *Pool) Add(cfg *agent.AgentConfig) (added bool) {
	_, loaded := pool.agents.LoadOrCompute(cfg.Addr, func() (*Agent, bool) {
		return newAgent(cfg), false
	})
	return !loaded
}

func (pool *Pool) Has(cfg *agent.AgentConfig) bool {
	_, ok := pool.agents.Load(cfg.Addr)
	return ok
}

func (pool *Pool) Remove(cfg *agent.AgentConfig) {
	pool.agents.Delete(cfg.Addr)
}

func (pool *Pool) RemoveAll() {
	pool.agents.Clear()
}

func (pool *Pool) List() []*Agent {
	agents := make([]*Agent, 0, pool.agents.Size())
	for _, agent := range pool.agents.Range {
		agents = append(agents, agent)
	}
	return agents
}

func (pool *Pool) Iter() iter.Seq2[string, *Agent] {
	return pool.agents.Range
}

func (pool *Pool) Num() int {
	return pool.agents.Size()
}

func (pool *Pool) getAgentByAddr(addr string) (agent *Agent, ok bool) {
	agent, ok = pool.agents.Load(addr)
	return agent, ok
}

type poolContextKey struct{}

func SetCtx(target interface{ SetValue(any, any) }, pool *Pool) {
	target.SetValue(poolContextKey{}, pool)
}

func FromCtx(ctx context.Context) *Pool {
	pool, _ := ctx.Value(poolContextKey{}).(*Pool)
	return pool
}
