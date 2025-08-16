package provider

import (
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/types"
)

func (p *Provider) Statistics() types.ProviderStats {
	var rps, streams types.RouteStats
	for _, r := range p.routes {
		switch r.Type() {
		case route.RouteTypeHTTP:
			rps.Add(r)
		case route.RouteTypeStream:
			streams.Add(r)
		}
	}
	return types.ProviderStats{
		Total:   rps.Total + streams.Total,
		RPs:     rps,
		Streams: streams,
		Type:    p.t,
	}
}
