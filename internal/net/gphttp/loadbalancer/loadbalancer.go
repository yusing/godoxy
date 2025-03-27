package loadbalancer

import (
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

// TODO: stats of each server.
// TODO: support weighted mode.
type (
	impl interface {
		ServeHTTP(srvs Servers, rw http.ResponseWriter, r *http.Request)
		OnAddServer(srv Server)
		OnRemoveServer(srv Server)
	}

	LoadBalancer struct {
		impl
		*Config

		task *task.Task

		pool   Pool
		poolMu sync.Mutex

		sumWeight Weight
		startTime time.Time

		l zerolog.Logger
	}
)

const maxWeight Weight = 100

func New(cfg *Config) *LoadBalancer {
	lb := &LoadBalancer{
		Config: new(Config),
		pool:   types.NewServerPool(),
		l:      logging.With().Str("name", cfg.Link).Logger(),
	}
	lb.UpdateConfigIfNeeded(cfg)
	return lb
}

// Start implements task.TaskStarter.
func (lb *LoadBalancer) Start(parent task.Parent) gperr.Error {
	lb.startTime = time.Now()
	lb.task = parent.Subtask("loadbalancer."+lb.Link, false)
	parent.OnCancel("lb_remove_route", func() {
		routes.DeleteHTTPRoute(lb.Link)
	})
	lb.task.OnFinished("cleanup", func() {
		if lb.impl != nil {
			lb.pool.RangeAll(func(k string, v Server) {
				lb.impl.OnRemoveServer(v)
			})
		}
	})
	return nil
}

// Task implements task.TaskStarter.
func (lb *LoadBalancer) Task() *task.Task {
	return lb.task
}

// Finish implements task.TaskFinisher.
func (lb *LoadBalancer) Finish(reason any) {
	lb.task.Finish(reason)
}

func (lb *LoadBalancer) updateImpl() {
	switch lb.Mode {
	case types.ModeUnset, types.ModeRoundRobin:
		lb.impl = lb.newRoundRobin()
	case types.ModeLeastConn:
		lb.impl = lb.newLeastConn()
	case types.ModeIPHash:
		lb.impl = lb.newIPHash()
	default: // should happen in test only
		lb.impl = lb.newRoundRobin()
	}
	lb.pool.RangeAll(func(_ string, srv Server) {
		lb.impl.OnAddServer(srv)
	})
}

func (lb *LoadBalancer) UpdateConfigIfNeeded(cfg *Config) {
	if cfg != nil {
		lb.poolMu.Lock()
		defer lb.poolMu.Unlock()

		lb.Link = cfg.Link

		if lb.Mode == types.ModeUnset && cfg.Mode != types.ModeUnset {
			lb.Mode = cfg.Mode
			if !lb.Mode.ValidateUpdate() {
				lb.l.Error().Msgf("invalid mode %q, fallback to %q", cfg.Mode, lb.Mode)
			}
			lb.updateImpl()
		}

		if len(lb.Options) == 0 && len(cfg.Options) > 0 {
			lb.Options = cfg.Options
		}
	}

	if lb.impl == nil {
		lb.updateImpl()
	}
}

func (lb *LoadBalancer) AddServer(srv Server) {
	lb.poolMu.Lock()
	defer lb.poolMu.Unlock()

	if lb.pool.Has(srv.Key()) { // FIXME: this should be a warning
		old, _ := lb.pool.Load(srv.Key())
		lb.sumWeight -= old.Weight()
		lb.impl.OnRemoveServer(old)
	}
	lb.pool.Store(srv.Key(), srv)
	lb.sumWeight += srv.Weight()

	lb.rebalance()
	lb.impl.OnAddServer(srv)

	lb.l.Debug().
		Str("action", "add").
		Str("server", srv.Name()).
		Msgf("%d servers available", lb.pool.Size())
}

func (lb *LoadBalancer) RemoveServer(srv Server) {
	lb.poolMu.Lock()
	defer lb.poolMu.Unlock()

	if !lb.pool.Has(srv.Key()) {
		return
	}

	lb.pool.Delete(srv.Key())

	lb.sumWeight -= srv.Weight()
	lb.rebalance()
	lb.impl.OnRemoveServer(srv)

	lb.l.Debug().
		Str("action", "remove").
		Str("server", srv.Name()).
		Msgf("%d servers left", lb.pool.Size())

	if lb.pool.Size() == 0 {
		lb.task.Finish("no server left")
		return
	}
}

func (lb *LoadBalancer) rebalance() {
	if lb.sumWeight == maxWeight {
		return
	}

	poolSize := lb.pool.Size()
	if poolSize == 0 {
		return
	}
	if lb.sumWeight == 0 { // distribute evenly
		weightEach := maxWeight / Weight(poolSize)
		remainder := maxWeight % Weight(poolSize)
		lb.pool.RangeAll(func(_ string, s Server) {
			w := weightEach
			lb.sumWeight += weightEach
			if remainder > 0 {
				w++
				remainder--
			}
			s.SetWeight(w)
		})
		return
	}

	// scale evenly
	scaleFactor := float64(maxWeight) / float64(lb.sumWeight)
	lb.sumWeight = 0

	lb.pool.RangeAll(func(_ string, s Server) {
		s.SetWeight(Weight(float64(s.Weight()) * scaleFactor))
		lb.sumWeight += s.Weight()
	})

	delta := maxWeight - lb.sumWeight
	if delta == 0 {
		return
	}
	lb.pool.Range(func(_ string, s Server) bool {
		if delta == 0 {
			return false
		}
		if delta > 0 {
			s.SetWeight(s.Weight() + 1)
			lb.sumWeight++
			delta--
		} else {
			s.SetWeight(s.Weight() - 1)
			lb.sumWeight--
			delta++
		}
		return true
	})
}

func (lb *LoadBalancer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	srvs := lb.availServers()
	if len(srvs) == 0 {
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Header.Get(httpheaders.HeaderGoDoxyCheckRedirect) != "" {
		// wake all servers
		for _, srv := range srvs {
			if err := srv.TryWake(); err != nil {
				lb.l.Warn().Err(err).
					Str("server", srv.Name()).
					Msg("failed to wake server")
			}
		}
	}
	lb.impl.ServeHTTP(srvs, rw, r)
}

// MarshalJSON implements health.HealthMonitor.
func (lb *LoadBalancer) MarshalJSON() ([]byte, error) {
	extra := make(map[string]any)
	lb.pool.RangeAll(func(k string, v Server) {
		extra[v.Key()] = v
	})

	return (&monitor.JSONRepresentation{
		Name:    lb.Name(),
		Status:  lb.Status(),
		Started: lb.startTime,
		Uptime:  lb.Uptime(),
		Extra: map[string]any{
			"config": lb.Config,
			"pool":   extra,
		},
	}).MarshalJSON()
}

// Name implements health.HealthMonitor.
func (lb *LoadBalancer) Name() string {
	return lb.Link
}

// Status implements health.HealthMonitor.
func (lb *LoadBalancer) Status() health.Status {
	if lb.pool.Size() == 0 {
		return health.StatusUnknown
	}

	isHealthy := true
	lb.pool.Range(func(_ string, srv Server) bool {
		if srv.Status().Bad() {
			isHealthy = false
			return false
		}
		return true
	})
	if !isHealthy {
		return health.StatusUnhealthy
	}
	return health.StatusHealthy
}

// Uptime implements health.HealthMonitor.
func (lb *LoadBalancer) Uptime() time.Duration {
	return time.Since(lb.startTime)
}

// Latency implements health.HealthMonitor.
func (lb *LoadBalancer) Latency() time.Duration {
	var sum time.Duration
	lb.pool.RangeAll(func(_ string, srv Server) {
		sum += srv.Latency()
	})
	return sum
}

// String implements health.HealthMonitor.
func (lb *LoadBalancer) String() string {
	return lb.Name()
}

func (lb *LoadBalancer) availServers() []Server {
	avail := make([]Server, 0, lb.pool.Size())
	lb.pool.RangeAll(func(_ string, srv Server) {
		if srv.Status().Good() {
			avail = append(avail, srv)
		}
	})
	return avail
}
