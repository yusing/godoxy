package provider

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/watcher"
)

type AgentProvider struct {
	*agent.AgentConfig
	docker      ProviderImpl
	initialized atomic.Pointer[agent.AgentConfig]
}

func (p *AgentProvider) ShortName() string {
	if cfg := p.initialized.Load(); cfg != nil {
		return cfg.Name
	}
	return p.AgentConfig.Name
}

func (p *AgentProvider) NewWatcher() watcher.Watcher {
	return p.docker.NewWatcher()
}

func (p *AgentProvider) IsExplicitOnly() bool {
	return p.docker.IsExplicitOnly()
}

func (p *AgentProvider) loadRoutesImpl(ctx context.Context) (route.Routes, error) {
	pool := agentpool.FromCtx(ctx)
	if pool == nil {
		return nil, errors.New("agent pool not initialized")
	}
	var cfg *agent.AgentConfig
	if initialized, ok := pool.Get(p.Addr); ok {
		cfg = initialized.AgentConfig
	} else {
		// Never mutate the config published through the provider registry.
		// Init writes certificates and discovery metadata, even on failure.
		cfg = &agent.AgentConfig{Addr: p.Addr}
		if err := cfg.Init(ctx); err != nil {
			return nil, err
		}
		pool.Add(cfg)
	}
	// Publish only complete discovery metadata, before routes capture ShortName.
	p.initialized.Store(cfg)
	return p.docker.loadRoutesImpl(ctx)
}

func (p *AgentProvider) Logger() *zerolog.Logger {
	l := log.With().Str("type", "docker").Str("name", p.ShortName()).Logger()
	return &l
}
