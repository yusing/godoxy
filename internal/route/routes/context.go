package routes

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"unsafe"

	"github.com/yusing/godoxy/internal/types"
)

type RouteContextKey struct{}

type RouteContext struct {
	context.Context

	Route types.HTTPRoute
}

var routeContextKey = RouteContextKey{}

func (r *RouteContext) Value(key any) any {
	if key == routeContextKey {
		return r.Route
	}
	return r.Context.Value(key)
}

func WithRouteContext(r *http.Request, route types.HTTPRoute) *http.Request {
	// we don't want to copy the request object every fucking requests
	// return r.WithContext(context.WithValue(r.Context(), routeContextKey, route))
	ctxFieldPtr := (*context.Context)(unsafe.Pointer(uintptr(unsafe.Pointer(r)) + ctxFieldOffset))
	*ctxFieldPtr = &RouteContext{
		Context: r.Context(),
		Route:   route,
	}
	return r
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

func TryGetUpstreamHostPort(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Host
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

var ctxFieldOffset uintptr

func init() {
	f, ok := reflect.TypeFor[http.Request]().FieldByName("ctx")
	if !ok {
		panic("ctx field not found")
	}
	ctxFieldOffset = f.Offset
}
