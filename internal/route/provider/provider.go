package provider

import (
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/route/provider/types"
	"github.com/yusing/go-proxy/internal/task"
	W "github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type (
	Provider struct {
		ProviderImpl

		t      types.ProviderType
		routes route.Routes

		watcher W.Watcher
	}
	ProviderImpl interface {
		fmt.Stringer
		ShortName() string
		IsExplicitOnly() bool
		loadRoutesImpl() (route.Routes, gperr.Error)
		NewWatcher() W.Watcher
		Logger() *zerolog.Logger
	}
)

const (
	providerEventFlushInterval = 300 * time.Millisecond
)

var ErrEmptyProviderName = errors.New("empty provider name")

func newProvider(t types.ProviderType) *Provider {
	return &Provider{t: t}
}

func NewFileProvider(filename string) (p *Provider, err error) {
	name := path.Base(filename)
	if name == "" {
		return nil, ErrEmptyProviderName
	}
	p = newProvider(types.ProviderTypeFile)
	p.ProviderImpl, err = FileProviderImpl(filename)
	if err != nil {
		return nil, err
	}
	p.watcher = p.NewWatcher()
	return
}

func NewDockerProvider(name string, dockerHost string) *Provider {
	p := newProvider(types.ProviderTypeDocker)
	p.ProviderImpl = DockerProviderImpl(name, dockerHost)
	p.watcher = p.NewWatcher()
	return p
}

func NewAgentProvider(cfg *agent.AgentConfig) *Provider {
	p := newProvider(types.ProviderTypeAgent)
	agent := &AgentProvider{
		AgentConfig: cfg,
		docker:      DockerProviderImpl(cfg.Name(), cfg.FakeDockerHost()),
	}
	p.ProviderImpl = agent
	p.watcher = p.NewWatcher()
	return p
}

func (p *Provider) GetType() types.ProviderType {
	return p.t
}

// to work with json marshaller.
func (p *Provider) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

func (p *Provider) startRoute(parent task.Parent, r *route.Route) gperr.Error {
	err := r.Start(parent)
	if err != nil {
		delete(p.routes, r.Alias)
		return err.Subject(r.Alias)
	}
	p.routes[r.Alias] = r
	return nil
}

// Start implements task.TaskStarter.
func (p *Provider) Start(parent task.Parent) gperr.Error {
	t := parent.Subtask("provider."+p.String(), false)

	errs := gperr.NewBuilder("routes error")
	for _, r := range p.routes {
		errs.Add(p.startRoute(t, r))
	}

	eventQueue := events.NewEventQueue(
		t.Subtask("event_queue", false),
		providerEventFlushInterval,
		func(events []events.Event) {
			handler := p.newEventHandler()
			// routes' lifetime should follow the provider's lifetime
			handler.Handle(t, events)
			handler.Log()
		},
		func(err gperr.Error) {
			gperr.LogError("event error", err, p.Logger())
		},
	)
	eventQueue.Start(p.watcher.Events(t.Context()))

	if err := errs.Error(); err != nil {
		return err.Subject(p.String())
	}
	return nil
}

func (p *Provider) RangeRoutes(do func(string, *route.Route)) {
	for alias, r := range p.routes {
		do(alias, r)
	}
}

func (p *Provider) GetRoute(alias string) (r *route.Route, ok bool) {
	r, ok = p.routes[alias]
	return
}

func (p *Provider) loadRoutes() (routes route.Routes, err gperr.Error) {
	routes, err = p.loadRoutesImpl()
	if err != nil && len(routes) == 0 {
		return route.Routes{}, err
	}
	errs := gperr.NewBuilder("routes error")
	errs.Add(err)
	// check for exclusion
	// set alias and provider, then validate
	for alias, r := range routes {
		r.Alias = alias
		r.Provider = p.ShortName()
		if err := r.Validate(); err != nil {
			errs.Add(err.Subject(alias))
			delete(routes, alias)
			continue
		}
		if r.ShouldExclude() {
			delete(routes, alias)
			continue
		}
		r.FinalizeHomepageConfig()
	}
	return routes, errs.Error()
}

func (p *Provider) LoadRoutes() (err gperr.Error) {
	p.routes, err = p.loadRoutes()
	return
}

func (p *Provider) NumRoutes() int {
	return len(p.routes)
}
