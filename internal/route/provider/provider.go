package provider

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route"
	provider "github.com/yusing/go-proxy/internal/route/provider/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	W "github.com/yusing/go-proxy/internal/watcher"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type (
	Provider struct {
		ProviderImpl

		t        provider.Type
		routes   route.Routes
		routesMu sync.RWMutex

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

var _ routes.Provider = (*Provider)(nil)

func newProvider(t provider.Type) *Provider {
	return &Provider{t: t}
}

func NewFileProvider(filename string) (p *Provider, err error) {
	name := path.Base(filename)
	if name == "" {
		return nil, ErrEmptyProviderName
	}
	p = newProvider(provider.ProviderTypeFile)
	p.ProviderImpl, err = FileProviderImpl(filename)
	if err != nil {
		return nil, err
	}
	p.watcher = p.NewWatcher()
	return
}

func NewDockerProvider(name string, dockerHost string) *Provider {
	p := newProvider(provider.ProviderTypeDocker)
	p.ProviderImpl = DockerProviderImpl(name, dockerHost)
	p.watcher = p.NewWatcher()
	return p
}

func NewAgentProvider(cfg *agent.AgentConfig) *Provider {
	p := newProvider(provider.ProviderTypeAgent)
	agent := &AgentProvider{
		AgentConfig: cfg,
		docker:      DockerProviderImpl(cfg.Name(), cfg.FakeDockerHost()),
	}
	p.ProviderImpl = agent
	p.watcher = p.NewWatcher()
	return p
}

func (p *Provider) GetType() provider.Type {
	return p.t
}

// to work with json marshaller.
func (p *Provider) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// Start implements task.TaskStarter.
func (p *Provider) Start(parent task.Parent) gperr.Error {
	errs := gperr.NewBuilder("routes error")
	errs.EnableConcurrency()

	t := parent.Subtask("provider."+p.String(), false)

	// no need to lock here because we are not modifying the routes map.
	routeSlice := make([]*route.Route, 0, len(p.routes))
	for _, r := range p.routes {
		routeSlice = append(routeSlice, r)
	}

	var wg sync.WaitGroup
	for _, r := range routeSlice {
		wg.Add(1)
		go func(r *route.Route) {
			defer wg.Done()
			errs.Add(p.startRoute(t, r))
		}(r)
	}
	wg.Wait()

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

func (p *Provider) LoadRoutes() (err gperr.Error) {
	p.routes, err = p.loadRoutes()
	return
}

func (p *Provider) NumRoutes() int {
	return len(p.routes)
}

func (p *Provider) IterRoutes(yield func(string, routes.Route) bool) {
	routes := p.lockCloneRoutes()
	for alias, r := range routes {
		if !yield(alias, r.Impl()) {
			break
		}
	}
}

func (p *Provider) FindService(project, service string) (routes.Route, bool) {
	switch p.GetType() {
	case provider.ProviderTypeDocker, provider.ProviderTypeAgent:
	default:
		return nil, false
	}
	if project == "" || service == "" {
		return nil, false
	}
	routes := p.lockCloneRoutes()
	for _, r := range routes {
		cont := r.ContainerInfo()
		if cont.DockerComposeProject() != project {
			continue
		}
		if cont.DockerComposeService() == service {
			return r.Impl(), true
		}
	}
	return nil, false
}

func (p *Provider) GetRoute(alias string) (routes.Route, bool) {
	r, ok := p.lockGetRoute(alias)
	if !ok {
		return nil, false
	}
	return r.Impl(), true
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
		r.SetProvider(p)
		if err := r.Validate(); err != nil {
			errs.Add(err.Subject(alias))
			delete(routes, alias)
			continue
		}
		r.FinalizeHomepageConfig()
	}
	return routes, errs.Error()
}

func (p *Provider) startRoute(parent task.Parent, r *route.Route) gperr.Error {
	err := r.Start(parent)
	if err != nil {
		p.lockDeleteRoute(r.Alias)
		return err.Subject(r.Alias)
	}
	p.lockAddRoute(r)
	r.Task().OnCancel("remove_route_from_provider", func() {
		p.lockDeleteRoute(r.Alias)
	})
	return nil
}

func (p *Provider) lockAddRoute(r *route.Route) {
	p.routesMu.Lock()
	defer p.routesMu.Unlock()
	p.routes[r.Alias] = r
}

func (p *Provider) lockDeleteRoute(alias string) {
	p.routesMu.Lock()
	defer p.routesMu.Unlock()
	delete(p.routes, alias)
}

func (p *Provider) lockGetRoute(alias string) (*route.Route, bool) {
	p.routesMu.RLock()
	defer p.routesMu.RUnlock()
	r, ok := p.routes[alias]
	return r, ok
}

func (p *Provider) lockCloneRoutes() route.Routes {
	p.routesMu.RLock()
	defer p.routesMu.RUnlock()
	return maps.Clone(p.routes)
}
