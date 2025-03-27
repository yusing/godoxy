package provider

import (
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/route"
	"github.com/yusing/go-proxy/internal/route/provider/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher"
)

type EventHandler struct {
	provider *Provider

	errs    *gperr.Builder
	added   *gperr.Builder
	removed *gperr.Builder
	updated *gperr.Builder
}

func (p *Provider) newEventHandler() *EventHandler {
	return &EventHandler{
		provider: p,
		errs:     gperr.NewBuilder("event errors"),
		added:    gperr.NewBuilder("added"),
		removed:  gperr.NewBuilder("removed"),
		updated:  gperr.NewBuilder("updated"),
	}
}

func (handler *EventHandler) Handle(parent task.Parent, events []watcher.Event) {
	oldRoutes := handler.provider.routes
	newRoutes, err := handler.provider.loadRoutes()
	if err != nil {
		handler.errs.Add(err)
		if len(newRoutes) == 0 {
			return
		}
	}

	if common.IsDebug {
		eventsLog := E.NewBuilder("events")
		for _, event := range events {
			eventsLog.Addf("event %s, actor: name=%s, id=%s", event.Action, event.ActorName, event.ActorID)
		}
		E.LogDebug(eventsLog.About(), eventsLog.Error(), handler.provider.Logger())

		oldRoutesLog := E.NewBuilder("old routes")
		for k := range oldRoutes {
			oldRoutesLog.Adds(k)
		}
		E.LogDebug(oldRoutesLog.About(), oldRoutesLog.Error(), handler.provider.Logger())

		newRoutesLog := E.NewBuilder("new routes")
		for k := range newRoutes {
			newRoutesLog.Adds(k)
		}
		E.LogDebug(newRoutesLog.About(), newRoutesLog.Error(), handler.provider.Logger())
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
	case types.ProviderTypeDocker:
		return route.Container.ContainerID == event.ActorID ||
			route.Container.ContainerName == event.ActorName
	case types.ProviderTypeFile:
		return true
	}
	// should never happen
	return false
}

func (handler *EventHandler) Add(parent task.Parent, route *route.Route) {
	err := handler.provider.startRoute(parent, route)
	if err != nil {
		handler.errs.Add(err.Subject("add"))
	} else {
		handler.added.Adds(route.Alias)
	}
}

func (handler *EventHandler) Remove(route *route.Route) {
	route.Finish("route removed")
	delete(handler.provider.routes, route.Alias)
	handler.removed.Adds(route.Alias)
}

func (handler *EventHandler) Update(parent task.Parent, oldRoute *route.Route, newRoute *route.Route) {
	oldRoute.Finish("route update")
	err := handler.provider.startRoute(parent, newRoute)
	if err != nil {
		handler.errs.Add(err.Subject("update"))
	} else {
		handler.updated.Adds(newRoute.Alias)
	}
}

func (handler *EventHandler) Log() {
	results := gperr.NewBuilder("event occurred")
	results.AddFrom(handler.added, false)
	results.AddFrom(handler.removed, false)
	results.AddFrom(handler.updated, false)
	results.AddFrom(handler.errs, false)
	if result := results.String(); result != "" {
		handler.provider.Logger().Info().Msg(result)
	}
}
