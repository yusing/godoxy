package loadbalancer

import (
	"net/http"
	"sync/atomic"

	F "github.com/yusing/go-proxy/internal/utils/functional"
)

type leastConn struct {
	*LoadBalancer
	nConn F.Map[Server, *atomic.Int64]
}

func (lb *LoadBalancer) newLeastConn() impl {
	return &leastConn{
		LoadBalancer: lb,
		nConn:        F.NewMapOf[Server, *atomic.Int64](),
	}
}

func (impl *leastConn) OnAddServer(srv Server) {
	impl.nConn.Store(srv, new(atomic.Int64))
}

func (impl *leastConn) OnRemoveServer(srv Server) {
	impl.nConn.Delete(srv)
}

func (impl *leastConn) ServeHTTP(srvs Servers, rw http.ResponseWriter, r *http.Request) {
	srv := srvs[0]
	minConn, ok := impl.nConn.Load(srv)
	if !ok {
		impl.l.Error().Msgf("[BUG] server %s not found", srv.Name())
		http.Error(rw, "Internal error", http.StatusInternalServerError)
	}

	for i := 1; i < len(srvs); i++ {
		nConn, ok := impl.nConn.Load(srvs[i])
		if !ok {
			impl.l.Error().Msgf("[BUG] server %s not found", srv.Name())
			http.Error(rw, "Internal error", http.StatusInternalServerError)
		}
		if nConn.Load() < minConn.Load() {
			minConn = nConn
			srv = srvs[i]
		}
	}

	minConn.Add(1)
	srv.ServeHTTP(rw, r)
	minConn.Add(-1)
}
