package provider

import (
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/task"
)

type EventHandler struct {
	provider *Provider

	errs gperr.Builder
}

func (p *Provider) newEventHandler() *EventHandler {
	return &EventHandler{
		provider: p,
		errs:     gperr.NewBuilder("event errors"),
	}
}

func (handler *EventHandler) Handle(parent task.Parent, events []watcher.Event) {
	oldRoutes := handler.provider.lockCloneRoutes()
	forceReload := hasForceReload(events)

	newRoutes, err := handler.provider.loadRoutes()
	if err != nil {
		handler.errs.Add(err)
		if len(newRoutes) == 0 {
			return
		}
	}

	for k, oldr := range oldRoutes {
		newr, ok := newRoutes[k]
		switch {
		case !ok:
			handler.Remove(oldr)
		case handler.shouldUpdateRoute(forceReload, events, newr):
			handler.Update(parent, oldr, newr)
		}
	}
	for k, newr := range newRoutes {
		if _, ok := oldRoutes[k]; !ok {
			handler.Add(parent, newr)
		}
	}
}

func (handler *EventHandler) shouldUpdateRoute(forceReload bool, events []watcher.Event, route *route.Route) bool {
	return forceReload || handler.matchAny(events, route)
}

func hasForceReload(events []watcher.Event) bool {
	for _, event := range events {
		if event.Action == watcherEvents.ActionForceReload {
			return true
		}
	}
	return false
}

func (handler *EventHandler) matchAny(events []watcher.Event, rt *route.Route) bool {
	for _, event := range events {
		if handler.match(event, rt) {
			return true
		}
	}
	return false
}

func (handler *EventHandler) match(event watcher.Event, rt *route.Route) bool {
	switch handler.provider.GetType() {
	case routing.ProviderTypeDocker, routing.ProviderTypeAgent:
		return rt.Container.ContainerID == event.ActorID ||
			rt.Container.ContainerName == event.ActorName
	case routing.ProviderTypeFile:
		return true
	}
	// should never happen
	return false
}

func (handler *EventHandler) Add(parent task.Parent, route *route.Route) {
	err := handler.provider.startRoute(parent, route)
	if err != nil {
		handler.errs.AddSubjectf(err, "add")
	}
}

func (handler *EventHandler) Remove(route *route.Route) {
	route.FinishAndWait("route removed")
}

func (handler *EventHandler) Update(parent task.Parent, oldRoute *route.Route, newRoute *route.Route) {
	oldRoute.FinishAndWait("route update")
	err := handler.provider.startRoute(parent, newRoute)
	if err != nil {
		handler.errs.AddSubjectf(err, "update")
	}
}

func (handler *EventHandler) Log() {
	if err := handler.errs.Error(); err != nil {
		handler.provider.Logger().Error().Msg(err.Error())
	}
}
