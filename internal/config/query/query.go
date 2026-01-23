package statequery

import (
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/types"
)

type RouteProviderListResponse struct {
	ShortName string `json:"short_name"`
	FullName  string `json:"full_name"`
} // @name RouteProvider

func DumpRouteProviders() map[string]types.RouteProvider {
	state := config.ActiveState.Load()
	entries := make(map[string]types.RouteProvider, state.NumProviders())
	for _, p := range state.IterProviders() {
		entries[p.ShortName()] = p
	}
	return entries
}

func RouteProviderList() []RouteProviderListResponse {
	state := config.ActiveState.Load()
	list := make([]RouteProviderListResponse, 0, state.NumProviders())
	for _, p := range state.IterProviders() {
		list = append(list, RouteProviderListResponse{
			ShortName: p.ShortName(),
			FullName:  p.String(),
		})
	}
	return list
}
