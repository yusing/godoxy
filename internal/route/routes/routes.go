package routes

import (
	"github.com/yusing/go-proxy/internal/types"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

var (
	HTTP   = pool.New[types.HTTPRoute]("http_routes")
	Stream = pool.New[types.StreamRoute]("stream_routes")
	// All is a pool of all routes, including HTTP, Stream routes and also excluded routes.
	All = pool.New[types.Route]("all_routes")
)

func init() {
	All.DisableLog()
}

func Iter(yield func(r types.Route) bool) {
	for _, r := range All.Iter {
		if !yield(r) {
			break
		}
	}
}

func IterKV(yield func(alias string, r types.Route) bool) {
	for k, r := range All.Iter {
		if !yield(k, r) {
			break
		}
	}
}

func NumRoutes() int {
	return All.Size()
}

func Clear() {
	HTTP.Clear()
	Stream.Clear()
	All.Clear()
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
	return All.Get(alias)
}
