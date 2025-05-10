package routes

import (
	"github.com/yusing/go-proxy/internal/utils/pool"
)

var (
	HTTP   = pool.New[HTTPRoute]("http_routes")
	Stream = pool.New[StreamRoute]("stream_routes")
)

func Iter(yield func(r Route) bool) {
	for _, r := range HTTP.Iter {
		if !yield(r) {
			break
		}
	}
}

func IterKV(yield func(alias string, r Route) bool) {
	for k, r := range HTTP.Iter {
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

func GetHTTPRouteOrExact(alias, host string) (HTTPRoute, bool) {
	r, ok := HTTP.Get(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return HTTP.Get(host)
}

func Get(alias string) (Route, bool) {
	r, ok := HTTP.Get(alias)
	if ok {
		return r, true
	}
	return Stream.Get(alias)
}
