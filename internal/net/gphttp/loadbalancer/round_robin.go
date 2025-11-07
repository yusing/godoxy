package loadbalancer

import (
	"net/http"
	"sync/atomic"

	"github.com/yusing/godoxy/internal/types"
)

type roundRobin struct {
	index atomic.Uint32
}

var _ impl = (*roundRobin)(nil)

func (*LoadBalancer) newRoundRobin() impl                          { return &roundRobin{} }
func (lb *roundRobin) OnAddServer(srv types.LoadBalancerServer)    {}
func (lb *roundRobin) OnRemoveServer(srv types.LoadBalancerServer) {}

func (lb *roundRobin) ChooseServer(srvs types.LoadBalancerServers, r *http.Request) types.LoadBalancerServer {
	if len(srvs) == 0 {
		return nil
	}
	index := lb.index.Add(1) % uint32(len(srvs))
	if lb.index.Load() >= 2*uint32(len(srvs)) {
		lb.index.Store(0)
	}
	return srvs[index]
}
