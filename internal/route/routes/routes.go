package routes

import (
	"github.com/yusing/go-proxy/internal/utils/pool"
)

var (
	HTTP   = pool.New[HTTPRoute]("http_routes")
	Stream = pool.New[StreamRoute]("stream_routes")
	// All is a pool of all routes, including HTTP, Stream routes and also excluded routes.
	All = pool.New[Route]("all_routes")
)

func init() {
	All.DisableLog()
}

func Iter(yield func(r Route) bool) {
	for _, r := range All.Iter {
		if !yield(r) {
			break
		}
	}
}

func IterKV(yield func(alias string, r Route) bool) {
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

func GetHTTPRouteOrExact(alias, host string) (HTTPRoute, bool) {
	r, ok := HTTP.Get(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return HTTP.Get(host)
}

func Get(alias string) (Route, bool) {
	return All.Get(alias)
}
