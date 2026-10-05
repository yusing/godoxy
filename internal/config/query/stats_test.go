package statequery

import (
	"iter"
	"maps"
	"testing"

	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route/provider"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/task"
)

type statisticsTestState struct {
	config.State
	providers map[string]routing.Provider
}

func (s *statisticsTestState) IterProviders() iter.Seq2[string, routing.Provider] {
	return maps.All(s.providers)
}

func TestGetStatisticsRecoveredAgentNames(t *testing.T) {
	runtimeTask := task.GetTestTask(t)
	pool := agentpool.NewPool()
	agentpool.SetCtx(runtimeTask, pool)
	state := &statisticsTestState{providers: make(map[string]routing.Provider)}
	config.SetCtx(runtimeTask, state)

	agents := []struct {
		addr string
		name string
	}{
		{addr: "127.0.0.1:invalid-first", name: "recovered-first"},
		{addr: "127.0.0.1:invalid-second", name: "recovered-second"},
	}
	for _, a := range agents {
		configured := &agent.AgentConfig{Addr: a.addr}
		p := provider.NewAgentProvider(configured)
		state.providers[a.addr] = p
		if got := p.ShortName(); got != "" {
			t.Fatalf("provider starts with name %q, want unnamed", got)
		}

		// Model discovery succeeding after registration without touching the
		// published config. Invalid ports fail locally, without a live agent.
		initialized := &agent.AgentConfig{Addr: a.addr, AgentInfo: agent.AgentInfo{Name: a.name}}
		if !pool.Add(initialized) {
			t.Fatalf("agent %q was already in the pool", a.addr)
		}
		if err := p.LoadRoutes(runtimeTask.Context()); err == nil {
			t.Fatalf("agent %q: expected Docker listing to fail for invalid port", a.name)
		}
		if got := p.ShortName(); got != a.name {
			t.Errorf("recovered provider name = %q, want %q despite Docker listing failure", got, a.name)
		}
		if configured.Name != "" {
			t.Errorf("registered config name mutated to %q", configured.Name)
		}
	}

	stats := GetStatistics(runtimeTask.Context())
	if len(stats.Providers) != len(agents) {
		t.Errorf("statistics contain %d providers, want %d: %+v", len(stats.Providers), len(agents), stats.Providers)
	}
	for _, a := range agents {
		got, ok := stats.Providers[a.name]
		if !ok {
			t.Errorf("statistics missing recovered agent %q", a.name)
			continue
		}
		if got.Type != routing.ProviderTypeAgent {
			t.Errorf("provider %q type = %v, want agent", a.name, got.Type)
		}
	}
}
