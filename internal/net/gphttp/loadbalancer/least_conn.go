package loadbalancer

import (
	"net/http"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/godoxy/internal/types"
)

type leastConn struct {
	*LoadBalancer
	nConn *xsync.Map[types.LoadBalancerServer, *atomic.Int64]
}

var _ impl = (*leastConn)(nil)
var _ customServeHTTP = (*leastConn)(nil)

func (lb *LoadBalancer) newLeastConn() impl {
	return &leastConn{
		LoadBalancer: lb,
		nConn:        xsync.NewMap[types.LoadBalancerServer, *atomic.Int64](),
	}
}

func (impl *leastConn) OnAddServer(srv types.LoadBalancerServer) {
	impl.nConn.Store(srv, new(atomic.Int64))
}

func (impl *leastConn) OnRemoveServer(srv types.LoadBalancerServer) {
	impl.nConn.Delete(srv)
}

func (impl *leastConn) ServeHTTP(srvs types.LoadBalancerServers, rw http.ResponseWriter, r *http.Request) {
	srv := impl.ChooseServer(srvs, r)
	if srv == nil {
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	minConn, ok := impl.nConn.Load(srv)
	if !ok {
		impl.l.Error().Msgf("[BUG] server %s not found", srv.Name())
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		return
	}

	minConn.Add(1)
	srv.ServeHTTP(rw, r)
	minConn.Add(-1)
}

func (impl *leastConn) ChooseServer(srvs types.LoadBalancerServers, r *http.Request) types.LoadBalancerServer {
	if len(srvs) == 0 {
		return nil
	}

	srv := srvs[0]
	minConn, ok := impl.nConn.Load(srv)
	if !ok {
		return nil
	}

	for i := 1; i < len(srvs); i++ {
		nConn, ok := impl.nConn.Load(srvs[i])
		if !ok {
			continue
		}
		if nConn.Load() < minConn.Load() {
			minConn = nConn
			srv = srvs[i]
		}
	}

	return srv
}
