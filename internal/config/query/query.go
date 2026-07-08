package statequery

import (
	config "github.com/yusing/godoxy/internal/config/types"
)

type RouteProviderListResponse struct {
	ShortName string `json:"short_name"`
	FullName  string `json:"full_name"`
} // @name RouteProvider

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
