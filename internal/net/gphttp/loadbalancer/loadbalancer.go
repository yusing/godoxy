package loadbalancer

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/net/gphttp/httpheaders"
	"github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/pool"
	"github.com/yusing/go-proxy/internal/watcher/health"
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

		pool   pool.Pool[Server]
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
		pool:   pool.New[Server]("loadbalancer." + cfg.Link),
		l:      logging.With().Str("name", cfg.Link).Logger(),
	}
	lb.UpdateConfigIfNeeded(cfg)
	return lb
}

// Start implements task.TaskStarter.
func (lb *LoadBalancer) Start(parent task.Parent) gperr.Error {
	lb.startTime = time.Now()
	lb.task = parent.Subtask("loadbalancer."+lb.Link, true)
	lb.task.OnCancel("cleanup", func() {
		if lb.impl != nil {
			for _, srv := range lb.pool.Iter {
				lb.impl.OnRemoveServer(srv)
			}
		}
		lb.task.Finish(nil)
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
	for _, srv := range lb.pool.Iter {
		lb.impl.OnAddServer(srv)
	}
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

	if old, ok := lb.pool.Get(srv.Key()); ok { // FIXME: this should be a warning
		lb.sumWeight -= old.Weight()
		lb.impl.OnRemoveServer(old)
		lb.pool.Del(old)
	}
	lb.pool.Add(srv)
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

	if _, ok := lb.pool.Get(srv.Key()); !ok {
		return
	}

	lb.pool.Del(srv)

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
		for _, srv := range lb.pool.Iter {
			w := weightEach
			lb.sumWeight += weightEach
			if remainder > 0 {
				w++
				remainder--
			}
			srv.SetWeight(w)
		}
		return
	}

	// scale evenly
	scaleFactor := float64(maxWeight) / float64(lb.sumWeight)
	lb.sumWeight = 0

	for _, srv := range lb.pool.Iter {
		srv.SetWeight(Weight(float64(srv.Weight()) * scaleFactor))
		lb.sumWeight += srv.Weight()
	}

	delta := maxWeight - lb.sumWeight
	if delta == 0 {
		return
	}
	for _, srv := range lb.pool.Iter {
		if delta == 0 {
			break
		}
		if delta > 0 {
			srv.SetWeight(srv.Weight() + 1)
			lb.sumWeight++
			delta--
		} else {
			srv.SetWeight(srv.Weight() - 1)
			lb.sumWeight--
			delta++
		}
	}
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
	for _, srv := range lb.pool.Iter {
		extra[srv.Key()] = srv
	}

	status, numHealthy := lb.status()

	return (&health.JSONRepresentation{
		Name:    lb.Name(),
		Status:  status,
		Detail:  fmt.Sprintf("%d/%d servers are healthy", numHealthy, lb.pool.Size()),
		Started: lb.startTime,
		Uptime:  lb.Uptime(),
		Latency: lb.Latency(),
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
	status, _ := lb.status()
	return status
}

func (lb *LoadBalancer) status() (status health.Status, numHealthy int) {
	if lb.pool.Size() == 0 {
		return health.StatusUnknown, 0
	}

	// should be healthy if at least one server is healthy
	numHealthy = 0
	for _, srv := range lb.pool.Iter {
		if srv.Status().Good() {
			numHealthy++
		}
	}
	if numHealthy == 0 {
		return health.StatusUnhealthy, numHealthy
	}
	return health.StatusHealthy, numHealthy
}

// Uptime implements health.HealthMonitor.
func (lb *LoadBalancer) Uptime() time.Duration {
	return time.Since(lb.startTime)
}

// Latency implements health.HealthMonitor.
func (lb *LoadBalancer) Latency() time.Duration {
	var sum time.Duration
	for _, srv := range lb.pool.Iter {
		sum += srv.Latency()
	}
	return sum
}

// String implements health.HealthMonitor.
func (lb *LoadBalancer) String() string {
	return lb.Name()
}

func (lb *LoadBalancer) availServers() []Server {
	avail := make([]Server, 0, lb.pool.Size())
	for _, srv := range lb.pool.Iter {
		if srv.Status().Good() {
			avail = append(avail, srv)
		}
	}
	return avail
}
