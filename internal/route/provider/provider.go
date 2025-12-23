package provider

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"sync"
	"time"

	"github.com/moby/moby/client"
	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/route"
	provider "github.com/yusing/godoxy/internal/route/provider/types"
	"github.com/yusing/godoxy/internal/types"
	W "github.com/yusing/godoxy/internal/watcher"
	"github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/env"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
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

var _ types.RouteProvider = (*Provider)(nil)

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
	return p, err
}

func NewDockerProvider(name string, dockerCfg types.DockerProviderConfig) *Provider {
	if dockerCfg.URL == common.DockerHostFromEnv {
		dockerCfg.URL = env.GetEnvString("DOCKER_HOST", client.DefaultDockerHost)
	}

	p := newProvider(provider.ProviderTypeDocker)
	p.ProviderImpl = DockerProviderImpl(name, dockerCfg)
	p.watcher = p.NewWatcher()
	return p
}

func NewAgentProvider(cfg *agent.AgentConfig) *Provider {
	p := newProvider(provider.ProviderTypeAgent)
	agent := &AgentProvider{
		AgentConfig: cfg,
		docker: DockerProviderImpl(cfg.Name, types.DockerProviderConfig{
			URL: cfg.FakeDockerHost(),
		}),
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
	return err
}

func (p *Provider) NumRoutes() int {
	return len(p.routes)
}

func (p *Provider) IterRoutes(yield func(string, types.Route) bool) {
	routes := p.lockCloneRoutes()
	for alias, r := range routes {
		impl := r.Impl()
		if impl == nil {
			continue
		}
		if !yield(alias, impl) {
			break
		}
	}
}

func (p *Provider) FindService(project, service string) (types.Route, bool) {
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
		if docker.DockerComposeProject(cont) != project {
			continue
		}
		if docker.DockerComposeService(cont) == service {
			return r.Impl(), true
		}
	}
	return nil, false
}

func (p *Provider) GetRoute(alias string) (types.Route, bool) {
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
	if !r.ShouldExclude() {
		r.Task().OnCancel("remove_route_from_provider", func() {
			p.lockDeleteRoute(r.Alias)
		})
	}
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
