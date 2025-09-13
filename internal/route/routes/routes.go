package routes

import (
	"github.com/yusing/go-proxy/internal/types"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

var (
	HTTP   = pool.New[types.HTTPRoute]("http_routes")
	Stream = pool.New[types.StreamRoute]("stream_routes")
)

func Iter(yield func(r types.Route) bool) {
	for _, r := range HTTP.Iter {
		if !yield(r) {
			break
		}
	}
	for _, r := range Stream.Iter {
		if !yield(r) {
			break
		}
	}
}

func IterKV(yield func(alias string, r types.Route) bool) {
	for k, r := range HTTP.Iter {
		if !yield(k, r) {
			break
		}
	}
	for k, r := range Stream.Iter {
		if !yield(k, r) {
			break
		}
	}
}

func NumRoutes() int {
	return HTTP.Size() + Stream.Size()
}

func Clear() {
	HTTP.Clear()
	Stream.Clear()
}

func GetHTTPRouteOrExact(alias, host string) (types.HTTPRoute, bool) {
	r, ok := HTTP.Get(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return HTTP.Get(host)
}

func Get(alias string) (types.Route, bool) {
	if r, ok := HTTP.Get(alias); ok {
		return r, true
	}
	if r, ok := Stream.Get(alias); ok {
		return r, true
	}
	return nil, false
}
