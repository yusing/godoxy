package loadbalancer

import (
	"net"
	"net/http"
	"slices"
	"sync"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
)

type ipHash struct {
	*LoadBalancer

	realIP *middleware.Middleware
	pool   types.LoadBalancerServers
	mu     sync.Mutex
}

var _ impl = (*ipHash)(nil)
var _ customServeHTTP = (*ipHash)(nil)

func (lb *LoadBalancer) newIPHash() impl {
	impl := &ipHash{LoadBalancer: lb}
	if len(lb.Options) == 0 {
		return impl
	}
	var err gperr.Error
	impl.realIP, err = middleware.RealIP.New(lb.Options)
	if err != nil {
		gperr.LogError("invalid real_ip options, ignoring", err, &impl.l)
	}
	return impl
}

func (impl *ipHash) OnAddServer(srv types.LoadBalancerServer) {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	impl.pool = append(impl.pool, srv)
}

func (impl *ipHash) OnRemoveServer(srv types.LoadBalancerServer) {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	for i, s := range impl.pool {
		if s == srv {
			impl.pool = slices.Delete(impl.pool, i, 1)
			return
		}
	}
}

func (impl *ipHash) ServeHTTP(_ types.LoadBalancerServers, rw http.ResponseWriter, r *http.Request) {
	if impl.realIP != nil {
		// resolve real client IP
		if proceed := impl.realIP.TryModifyRequest(rw, r); !proceed {
			return
		}
	}

	srv := impl.ChooseServer(impl.pool, r)
	if srv == nil || srv.Status().Bad() {
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	srv.ServeHTTP(rw, r)
}

func (impl *ipHash) ChooseServer(_ types.LoadBalancerServers, r *http.Request) types.LoadBalancerServer {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	if len(impl.pool) == 0 {
		return nil
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	return impl.pool[xxhash3.HashString(ip)%uint64(len(impl.pool))]
}
