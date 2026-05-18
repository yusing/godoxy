package entrypoint

import (
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/types"
)

type wildcardRouteIndex struct {
	oneLabel map[string]types.HTTPRoute
}

func newWildcardRouteIndex() *wildcardRouteIndex {
	return &wildcardRouteIndex{
		oneLabel: make(map[string]types.HTTPRoute),
	}
}

func (idx *wildcardRouteIndex) Add(route types.HTTPRoute) {
	if suffix, ok := wildcardSuffix(route.Key()); ok {
		existingRoute, exists := idx.oneLabel[suffix]
		if exists && !preferWildcardRoute(route, existingRoute) {
			return
		}
		if exists {
			log.Warn().
				Str("suffix", suffix).
				Str("old_route", existingRoute.Key()).
				Str("new_route", route.Key()).
				Msg("replacing wildcard route with conflicting suffix")
		}
		idx.oneLabel[suffix] = route
	}
}

type routePreference interface {
	PreferOver(other any) bool
}

func preferWildcardRoute(route, existingRoute types.HTTPRoute) bool {
	if preferredRoute, ok := route.(routePreference); ok {
		return preferredRoute.PreferOver(existingRoute)
	}
	if preferredExisting, ok := existingRoute.(routePreference); ok {
		return !preferredExisting.PreferOver(route)
	}
	return true
}

func (idx *wildcardRouteIndex) Find(host string) types.HTTPRoute {
	if idx == nil {
		return nil
	}
	key, ok := wildcardLookupKey(host)
	if !ok {
		return nil
	}
	return idx.oneLabel[key]
}

func wildcardSuffix(alias string) (string, bool) {
	alias = strings.TrimSuffix(strings.ToLower(alias), ".")
	suffix, ok := strings.CutPrefix(alias, "*.")
	if !ok || suffix == "" || strings.Contains(suffix, "*") {
		return "", false
	}
	return suffix, true
}

func wildcardLookupKey(host string) (string, bool) {
	host, _, _ = strings.Cut(host, ":")
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	_, suffix, ok := strings.Cut(host, ".")
	if !ok || suffix == "" {
		return "", false
	}
	return suffix, true
}
