package loadbalancer

import (
	"net/http"
	"sync/atomic"

	"github.com/yusing/go-proxy/internal/types"
)

type roundRobin struct {
	index atomic.Uint32
}

func (*LoadBalancer) newRoundRobin() impl                          { return &roundRobin{} }
func (lb *roundRobin) OnAddServer(srv types.LoadBalancerServer)    {}
func (lb *roundRobin) OnRemoveServer(srv types.LoadBalancerServer) {}

func (lb *roundRobin) ServeHTTP(srvs types.LoadBalancerServers, rw http.ResponseWriter, r *http.Request) {
	index := lb.index.Add(1) % uint32(len(srvs))
	srvs[index].ServeHTTP(rw, r)
	if lb.index.Load() >= 2*uint32(len(srvs)) {
		lb.index.Store(0)
	}
}
