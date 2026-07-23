package provider

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/route"
	W "github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/goutils/task"
)

type StaticProvider struct {
	name   string
	routes route.Routes
	l      zerolog.Logger
}

func NewStaticProvider(name string, routes route.Routes) *Provider {
	p := newProvider("static")
	p.ProviderImpl = &StaticProvider{
		name:   name,
		routes: routes,
		l:      log.With().Str("type", "static").Str("name", name).Logger(),
	}
	p.watcher = p.NewWatcher()
	return p
}

func (p *StaticProvider) String() string { return p.name }

func (p *StaticProvider) ShortName() string { return p.name }

func (p *StaticProvider) IsExplicitOnly() bool { return false }

func (p *StaticProvider) Logger() *zerolog.Logger { return &p.l }

func (p *StaticProvider) loadRoutesImpl(context.Context) (route.Routes, error) { return p.routes, nil }

func (p *StaticProvider) NewWatcher() W.Watcher { return noopWatcher{} }

type noopWatcher struct{}

func (noopWatcher) Watch(task.Parent) W.Stream {
	eventCh := make(chan W.Event)
	errCh := make(chan error)
	close(eventCh)
	close(errCh)
	return W.Stream{Events: eventCh, Errors: errCh, Ready: W.Ready()}
}
