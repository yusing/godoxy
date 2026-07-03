package provider

import (
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routing"
)

func (p *Provider) Statistics() routing.ProviderStats {
	var rps, streams routing.RouteStats
	for _, r := range p.routes {
		switch r.Type() {
		case route.RouteTypeHTTP:
			rps.Add(r)
		case route.RouteTypeStream:
			streams.Add(r)
		}
	}
	return routing.ProviderStats{
		Total:   rps.Total + streams.Total,
		RPs:     rps,
		Streams: streams,
		Type:    p.t,
	}
}
