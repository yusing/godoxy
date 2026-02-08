package entrypoint

import (
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/types"
)

// httpPoolAdapter implements the PoolLike interface for the HTTP routes.
type httpPoolAdapter struct {
	ep *Entrypoint
}

func newHTTPPoolAdapter(ep *Entrypoint) httpPoolAdapter {
	return httpPoolAdapter{ep: ep}
}

func (h httpPoolAdapter) Iter(yield func(alias string, route types.HTTPRoute) bool) {
	for addr, srv := range h.ep.servers.Range {
		// default routes are added to both HTTP and HTTPS servers, we don't need to iterate over them twice.
		if addr == common.ProxyHTTPSAddr {
			continue
		}
		for alias, route := range srv.routes.Iter {
			if !yield(alias, route) {
				return
			}
		}
	}
}

func (h httpPoolAdapter) Get(alias string) (types.HTTPRoute, bool) {
	for addr, srv := range h.ep.servers.Range {
		if addr == common.ProxyHTTPSAddr {
			continue
		}
		if route, ok := srv.routes.Get(alias); ok {
			return route, true
		}
	}
	return nil, false
}

func (h httpPoolAdapter) Size() (n int) {
	for addr, srv := range h.ep.servers.Range {
		if addr == common.ProxyHTTPSAddr {
			continue
		}
		n += srv.routes.Size()
	}
	return
}
