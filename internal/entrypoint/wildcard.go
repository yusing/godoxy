package entrypoint

import (
	"strings"

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
		idx.oneLabel[suffix] = route
	}
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
