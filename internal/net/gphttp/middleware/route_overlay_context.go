package middleware

import (
	"context"
	"net/http"

	strutils "github.com/yusing/goutils/strings"
)

type routeOverlayConsumptionContextKey struct{}

type routeOverlayConsumption struct {
	bypass      map[string]struct{}
	middlewares map[string]struct{}
}

var routeOverlayConsumptionKey routeOverlayConsumptionContextKey

func WithConsumedRouteOverlays(
	r *http.Request,
	bypass map[string]struct{},
	middlewares map[string]struct{},
) *http.Request {
	if len(bypass) == 0 && len(middlewares) == 0 {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), routeOverlayConsumptionKey, routeOverlayConsumption{
		bypass:      bypass,
		middlewares: middlewares,
	}))
}

func isRouteBypassPromoted(r *http.Request, middlewareName string) bool {
	return routeOverlayConsumed(r, middlewareName, func(consumption routeOverlayConsumption) map[string]struct{} {
		return consumption.bypass
	})
}

func isRouteMiddlewareConsumed(r *http.Request, middlewareName string) bool {
	return routeOverlayConsumed(r, middlewareName, func(consumption routeOverlayConsumption) map[string]struct{} {
		return consumption.middlewares
	})
}

func routeOverlayConsumed(
	r *http.Request,
	middlewareName string,
	selectSet func(routeOverlayConsumption) map[string]struct{},
) bool {
	if r == nil {
		return false
	}
	consumption, ok := r.Context().Value(routeOverlayConsumptionKey).(routeOverlayConsumption)
	if !ok {
		return false
	}
	_, ok = selectSet(consumption)[strutils.ToLowerNoSnake(middlewareName)]
	return ok
}
