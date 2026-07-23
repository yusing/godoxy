package provider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/agent/pkg/agent"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/types"
	W "github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/eventqueue"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/task"
)

type (
	Provider struct {
		ProviderImpl

		t           routing.ProviderType
		routes      route.Routes
		routesMu    sync.RWMutex
		diagnostics config.LoadDiagnostics
		preparation routing.ProviderActivation

		watcher W.Watcher
	}
	ProviderImpl interface {
		fmt.Stringer
		ShortName() string
		IsExplicitOnly() bool
		loadRoutesImpl(context.Context) (route.Routes, error)
		NewWatcher() W.Watcher
		Logger() *zerolog.Logger
	}
)

const (
	providerEventFlushInterval = 300 * time.Millisecond
)

var ErrEmptyProviderName = errors.New("empty provider name")

var ErrWatcherStreamUnavailable = errors.New("watcher returned an incomplete stream")

var _ routing.Provider = (*Provider)(nil)

func newProvider(t routing.ProviderType) *Provider {
	return &Provider{t: t}
}

func NewFileProvider(filename string) (p *Provider, err error) {
	name := path.Base(filename)
	if name == "" {
		return nil, ErrEmptyProviderName
	}
	p = newProvider(routing.ProviderTypeFile)
	p.ProviderImpl, err = FileProviderImpl(filename)
	if err != nil {
		return nil, err
	}
	p.watcher = p.NewWatcher()
	return p, err
}

func NewDockerProvider(name string, dockerCfg types.DockerProviderConfig) *Provider {
	p := newProvider(routing.ProviderTypeDocker)
	p.ProviderImpl = DockerProviderImpl(name, dockerCfg)
	p.watcher = p.NewWatcher()
	return p
}

func NewAgentProvider(cfg *agent.AgentConfig) *Provider {
	p := newProvider(routing.ProviderTypeAgent)
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

func (p *Provider) GetType() routing.ProviderType {
	return p.t
}

// MarshalText implements encoding.TextMarshaler.
func (p *Provider) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// Activate starts every valid route, keeps successful siblings active when
// others fail, and starts the provider watcher so later changes can recover a
// partially failed initial load. An infrastructure failure is retained in the
// report; a zero-route provider without such a failure is ready.
func (p *Provider) Activate(parent task.Parent) routing.ProviderActivation {
	activation := p.preparation
	activation.Provider = p.String()
	activation.FailedRoutes = slices.Clone(activation.FailedRoutes)
	if state := config.FromCtx(parent.Context()); state != nil {
		p.diagnostics, _ = state.(config.LoadDiagnostics)
	}

	// no need to lock here because we are not modifying the routes map.
	routeSlice := make([]*route.Route, 0, len(p.routes))
	for _, r := range p.routes {
		routeSlice = append(routeSlice, r)
	}
	activation.AttemptedRoutes = len(routeSlice)
	if cause := context.Cause(parent.Context()); cause != nil {
		activation.AttemptedRoutes = 0
		activation.InfrastructureError = gperr.Join(activation.InfrastructureError, cause)
		return activation
	}
	t := parent.Subtask("provider."+p.String(), false)

	var wg sync.WaitGroup
	routeErrors := make([]error, len(routeSlice))
	for i, r := range routeSlice {
		wg.Go(func() {
			routeErrors[i] = p.startRoute(t, r)
		})
	}
	wg.Wait()
	for i, err := range routeErrors {
		if err == nil {
			activation.ActiveRoutes++
		} else {
			activation.FailedRoutes = append(activation.FailedRoutes, routing.RouteActivationIssue{
				Route: routeSlice[i].Alias,
				Err:   gperr.Wrap(err),
			})
		}
	}

	if cause := context.Cause(t.Context()); cause != nil {
		activation.InfrastructureError = gperr.Join(activation.InfrastructureError, cause)
		return activation
	}

	opts := eventqueue.Options[watcherEvents.Event]{
		FlushInterval: providerEventFlushInterval,
		OnFlush: func(evs []watcherEvents.Event) {
			handler := p.newEventHandler()
			// routes' lifetime should follow the provider's lifetime
			handler.Handle(t, evs)
			handler.Log()

			history := events.FromCtx(t.Context())
			if history == nil {
				return
			}
			globalEvents := make([]events.Event, len(evs))
			for i, ev := range evs {
				globalEvents[i] = events.NewEvent(events.LevelInfo, "provider_event", ev.Action.String(), map[string]any{
					"provider": p.String(),
					"type":     ev.Type,      // file / docker
					"actor":    ev.ActorName, // file path / container name
				})
			}
			history.AddAll(globalEvents)
		},
		OnError: func(err error) {
			p.Logger().Err(err).Msg("event error")
		},
	}
	watcherTask := t.Subtask("watcher", false)
	stream := p.watcher.Watch(watcherTask)
	if stream.Ready == nil || stream.Events == nil || stream.Errors == nil {
		watcherTask.FinishAndWait(ErrWatcherStreamUnavailable)
		activation.InfrastructureError = gperr.Join(activation.InfrastructureError, ErrWatcherStreamUnavailable)
		return activation
	}

	var readyErr error
	select {
	case readyErr = <-stream.Ready:
	case <-t.Context().Done():
		readyErr = context.Cause(t.Context())
	}
	if readyErr != nil {
		watcherTask.FinishAndWait(readyErr)
		activation.InfrastructureError = gperr.Join(activation.InfrastructureError, readyErr)
		return activation
	}
	if cause := context.Cause(t.Context()); cause != nil {
		watcherTask.FinishAndWait(cause)
		activation.InfrastructureError = gperr.Join(activation.InfrastructureError, cause)
		return activation
	}

	eventQueue := eventqueue.New(watcherTask.Subtask("event_queue", false), opts)
	eventQueue.Start(stream.Events, stream.Errors)
	activation.EventLoopReady = true
	return activation
}

func (p *Provider) LoadRoutes(ctx context.Context) (err error) {
	p.routes, err = p.loadRoutes(ctx)
	return err
}

func (p *Provider) loadRoutes(ctx context.Context) (route.Routes, error) {
	routes, infrastructureErr := p.loadRoutesImpl(ctx)
	if routes == nil {
		routes = make(route.Routes)
	}
	p.preparation = routing.ProviderActivation{
		Provider:            p.String(),
		DesiredRoutes:       len(routes),
		InfrastructureError: gperr.Wrap(infrastructureErr),
	}

	errs := gperr.NewBuilder("routes error")
	errs.Add(infrastructureErr)
	for alias, r := range routes {
		if r == nil {
			r = new(route.Route)
			routes[alias] = r
		}
		r.Alias = alias
		r.SetProvider(p)
		if err := r.ValidateContext(ctx); err != nil {
			subjectErr := gperr.PrependSubject(err, alias)
			errs.Add(subjectErr)
			p.preparation.FailedRoutes = append(p.preparation.FailedRoutes, routing.RouteActivationIssue{
				Route: alias,
				Err:   subjectErr,
			})
			delete(routes, alias)
		}
	}
	return routes, errs.Error()
}

func (p *Provider) NumRoutes() int {
	return len(p.routes)
}

func (p *Provider) IterRoutes(yield func(string, routing.Route) bool) {
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

func (p *Provider) FindService(project, service string) (routing.Route, bool) {
	switch p.GetType() {
	case routing.ProviderTypeDocker, routing.ProviderTypeAgent:
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

func (p *Provider) GetRoute(alias string) (routing.Route, bool) {
	r, ok := p.lockGetRoute(alias)
	if !ok {
		return nil, false
	}
	return r.Impl(), true
}

func (p *Provider) startRoute(parent task.Parent, r *route.Route) error {
	err := r.Start(parent)
	if err != nil {
		p.lockDeleteRoute(r.Alias)
		return gperr.PrependSubject(err, r.Alias)
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
