package provider

import (
	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/gperr"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/watcher"
)

type AgentProvider struct {
	*agent.AgentConfig
	docker ProviderImpl
}

func (p *AgentProvider) ShortName() string {
	return p.AgentConfig.Name
}

func (p *AgentProvider) NewWatcher() watcher.Watcher {
	return p.docker.NewWatcher()
}

func (p *AgentProvider) IsExplicitOnly() bool {
	return p.docker.IsExplicitOnly()
}

func (p *AgentProvider) loadRoutesImpl() (route.Routes, gperr.Error) {
	return p.docker.loadRoutesImpl()
}

func (p *AgentProvider) Logger() *zerolog.Logger {
	return p.docker.Logger()
}
