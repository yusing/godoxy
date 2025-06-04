package provider

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route"
	provider "github.com/yusing/go-proxy/internal/route/provider/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher"
	eventsPkg "github.com/yusing/go-proxy/internal/watcher/events"
)

type EventHandler struct {
	provider *Provider

	errs *gperr.Builder
}

func (p *Provider) newEventHandler() *EventHandler {
	return &EventHandler{
		provider: p,
		errs:     gperr.NewBuilder("event errors"),
	}
}

func (handler *EventHandler) Handle(parent task.Parent, events []watcher.Event) {
	oldRoutes := handler.provider.lockCloneRoutes()

	isForceReload := false
	for _, event := range events {
		if event.Action == eventsPkg.ActionForceReload {
			isForceReload = true
			break
		}
	}

	newRoutes, err := handler.provider.loadRoutes()
	if err != nil {
		handler.errs.Add(err)
		if len(newRoutes) == 0 && !isForceReload {
			return
		}
	}

	for k, oldr := range oldRoutes {
		newr, ok := newRoutes[k]
		switch {
		case !ok:
			handler.Remove(oldr)
		case handler.matchAny(events, newr):
			handler.Update(parent, oldr, newr)
		}
	}
	for k, newr := range newRoutes {
		if _, ok := oldRoutes[k]; !ok {
			handler.Add(parent, newr)
		}
	}
}

func (handler *EventHandler) matchAny(events []watcher.Event, route *route.Route) bool {
	for _, event := range events {
		if handler.match(event, route) {
			return true
		}
	}
	return false
}

func (handler *EventHandler) match(event watcher.Event, route *route.Route) bool {
	switch handler.provider.GetType() {
	case provider.ProviderTypeDocker, provider.ProviderTypeAgent:
		return route.Container.ContainerID == event.ActorID ||
			route.Container.ContainerName == event.ActorName
	case provider.ProviderTypeFile:
		return true
	}
	// should never happen
	return false
}

func (handler *EventHandler) Add(parent task.Parent, route *route.Route) {
	err := handler.provider.startRoute(parent, route)
	if err != nil {
		handler.errs.Add(err.Subject("add"))
	}
}

func (handler *EventHandler) Remove(route *route.Route) {
	route.FinishAndWait("route removed")
}

func (handler *EventHandler) Update(parent task.Parent, oldRoute *route.Route, newRoute *route.Route) {
	oldRoute.FinishAndWait("route update")
	err := handler.provider.startRoute(parent, newRoute)
	if err != nil {
		handler.errs.Add(err.Subject("update"))
	}
}

func (handler *EventHandler) Log() {
	if err := handler.errs.Error(); err != nil {
		handler.provider.Logger().Info().Msg(err.Error())
	}
}
