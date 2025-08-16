package routes

import (
	"context"
	"net/http"
	"net/url"

	"github.com/yusing/go-proxy/internal/types"
)

type RouteContext struct{}

var routeContextKey = RouteContext{}

func WithRouteContext(r *http.Request, route types.HTTPRoute) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), routeContextKey, route))
}

func TryGetRoute(r *http.Request) types.HTTPRoute {
	if route, ok := r.Context().Value(routeContextKey).(types.HTTPRoute); ok {
		return route
	}
	return nil
}

func tryGetURL(r *http.Request) *url.URL {
	if route := TryGetRoute(r); route != nil {
		u := route.TargetURL()
		if u != nil {
			return &u.URL
		}
	}
	return nil
}

func TryGetUpstreamName(r *http.Request) string {
	if route := TryGetRoute(r); route != nil {
		return route.Name()
	}
	return ""
}

func TryGetUpstreamScheme(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Scheme
	}
	return ""
}

func TryGetUpstreamHost(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Hostname()
	}
	return ""
}

func TryGetUpstreamPort(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Port()
	}
	return ""
}

func TryGetUpstreamAddr(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Host
	}
	return ""
}

func TryGetUpstreamURL(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.String()
	}
	return ""
}
