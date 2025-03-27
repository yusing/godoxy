package loadbalancer

import (
	"hash/fnv"
	"net"
	"net/http"
	"sync"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
)

type ipHash struct {
	*LoadBalancer

	realIP *middleware.Middleware
	pool   Servers
	mu     sync.Mutex
}

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

func (impl *ipHash) OnAddServer(srv Server) {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	for i, s := range impl.pool {
		if s == srv {
			return
		}
		if s == nil {
			impl.pool[i] = srv
			return
		}
	}

	impl.pool = append(impl.pool, srv)
}

func (impl *ipHash) OnRemoveServer(srv Server) {
	impl.mu.Lock()
	defer impl.mu.Unlock()

	for i, s := range impl.pool {
		if s == srv {
			impl.pool[i] = nil
			return
		}
	}
}

func (impl *ipHash) ServeHTTP(_ Servers, rw http.ResponseWriter, r *http.Request) {
	if impl.realIP != nil {
		impl.realIP.ModifyRequest(impl.serveHTTP, rw, r)
	} else {
		impl.serveHTTP(rw, r)
	}
}

func (impl *ipHash) serveHTTP(rw http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(rw, "Internal error", http.StatusInternalServerError)
		impl.l.Err(err).Msg("invalid remote address " + r.RemoteAddr)
		return
	}
	idx := hashIP(ip) % uint32(len(impl.pool))

	srv := impl.pool[idx]
	if srv == nil || srv.Status().Bad() {
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
	}
	srv.ServeHTTP(rw, r)
}

func hashIP(ip string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(ip))
	return h.Sum32()
}
