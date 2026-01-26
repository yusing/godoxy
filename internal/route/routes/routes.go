package routes

import (
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/pool"
)

var (
	HTTP   = pool.New[types.HTTPRoute]("http_routes")
	Stream = pool.New[types.StreamRoute]("stream_routes")

	Excluded = pool.New[types.Route]("excluded_routes")
)

func IterActive(yield func(r types.Route) bool) {
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

func IterAll(yield func(r types.Route) bool) {
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
	for _, r := range Excluded.Iter {
		if !yield(r) {
			break
		}
	}
}

func NumActiveRoutes() int {
	return HTTP.Size() + Stream.Size()
}

func NumAllRoutes() int {
	return HTTP.Size() + Stream.Size() + Excluded.Size()
}

func Clear() {
	HTTP.Clear()
	Stream.Clear()
	Excluded.Clear()
}

func GetHTTPRouteOrExact(alias, host string) (types.HTTPRoute, bool) {
	r, ok := HTTP.Get(alias)
	if ok {
		return r, true
	}
	// try find with exact match
	return HTTP.Get(host)
}

// Get returns the route with the given alias.
//
// It does not return excluded routes.
func Get(alias string) (types.Route, bool) {
	if r, ok := HTTP.Get(alias); ok {
		return r, true
	}
	if r, ok := Stream.Get(alias); ok {
		return r, true
	}
	return nil, false
}

// GetIncludeExcluded returns the route with the given alias, including excluded routes.
func GetIncludeExcluded(alias string) (types.Route, bool) {
	if r, ok := HTTP.Get(alias); ok {
		return r, true
	}
	if r, ok := Stream.Get(alias); ok {
		return r, true
	}
	return Excluded.Get(alias)
}
