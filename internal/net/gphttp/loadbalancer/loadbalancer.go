package loadbalancer

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

// TODO: stats of each server.
// TODO: support weighted mode.
type (
	impl interface {
		OnAddServer(srv types.LoadBalancerServer)
		OnRemoveServer(srv types.LoadBalancerServer)
		ChooseServer(srvs types.LoadBalancerServers, r *http.Request) types.LoadBalancerServer
	}
	customServeHTTP interface {
		ServeHTTP(srvs types.LoadBalancerServers, rw http.ResponseWriter, r *http.Request)
	}

	LoadBalancer struct {
		impl
		*types.LoadBalancerConfig

		task *task.Task

		pool   *pool.Pool[types.LoadBalancerServer]
		poolMu sync.Mutex

		sumWeight int
		startTime time.Time

		l zerolog.Logger
	}
)

const maxWeight int = 100

func New(cfg *types.LoadBalancerConfig) *LoadBalancer {
	lb := &LoadBalancer{
		LoadBalancerConfig: cfg,
		pool:               pool.New[types.LoadBalancerServer]("loadbalancer." + cfg.Link),
		l:                  log.With().Str("name", cfg.Link).Logger(),
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
	case types.LoadbalanceModeUnset, types.LoadbalanceModeRoundRobin:
		lb.impl = lb.newRoundRobin()
	case types.LoadbalanceModeLeastConn:
		lb.impl = lb.newLeastConn()
	case types.LoadbalanceModeIPHash:
		lb.impl = lb.newIPHash()
	default: // should happen in test only
		lb.impl = lb.newRoundRobin()
	}
	for _, srv := range lb.pool.Iter {
		lb.impl.OnAddServer(srv)
	}
}

func (lb *LoadBalancer) UpdateConfigIfNeeded(cfg *types.LoadBalancerConfig) {
	if cfg != nil {
		lb.poolMu.Lock()
		defer lb.poolMu.Unlock()

		lb.Link = cfg.Link

		if lb.Mode == types.LoadbalanceModeUnset && cfg.Mode != types.LoadbalanceModeUnset {
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

func (lb *LoadBalancer) AddServer(srv types.LoadBalancerServer) {
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
}

func (lb *LoadBalancer) RemoveServer(srv types.LoadBalancerServer) {
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
		weightEach := maxWeight / poolSize
		remainder := maxWeight % poolSize
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
		srv.SetWeight(int(float64(srv.Weight()) * scaleFactor))
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
	if r.URL.Path == idlewatcher.WakeEventsPath {
		var errs gperr.Group
		// wake all servers
		for _, srv := range srvs {
			errs.Go(func() error {
				err := srv.TryWake()
				if err != nil {
					return fmt.Errorf("failed to wake server %q: %w", srv.Name(), err)
				}
				return nil
			})
		}
		if err := errs.Wait().Error(); err != nil {
			gperr.LogWarn("failed to wake some servers", err, &lb.l)
		}
	}

	// Check for idlewatcher requests or sticky sessions
	if lb.Sticky || isIdlewatcherRequest(r) {
		if selectedServer := getStickyServer(r, srvs); selectedServer != nil {
			selectedServer.ServeHTTP(rw, r)
			return
		}
		// No sticky session, choose a server and set cookie
		selectedServer := lb.impl.ChooseServer(srvs, r)
		if selectedServer != nil {
			setStickyCookie(rw, r, selectedServer, lb.StickyMaxAge)
			selectedServer.ServeHTTP(rw, r)
			return
		}
	}

	if customServeHTTP, ok := lb.impl.(customServeHTTP); ok {
		customServeHTTP.ServeHTTP(srvs, rw, r)
		return
	}

	selectedServer := lb.ChooseServer(srvs, r)
	if selectedServer == nil {
		http.Error(rw, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	selectedServer.ServeHTTP(rw, r)
}

// MarshalJSON implements health.HealthMonitor.
func (lb *LoadBalancer) MarshalJSON() ([]byte, error) {
	extra := make(map[string]any)
	for _, srv := range lb.pool.Iter {
		extra[srv.Key()] = srv
	}

	status, numHealthy := lb.status()

	return (&types.HealthJSONRepr{
		Name:    lb.Name(),
		Status:  status,
		Detail:  fmt.Sprintf("%d/%d servers are healthy", numHealthy, lb.pool.Size()),
		Started: lb.startTime,
		Uptime:  lb.Uptime(),
		Latency: lb.Latency(),
		Extra: &types.HealthExtra{
			Config: lb.LoadBalancerConfig,
			Pool:   extra,
		},
	}).MarshalJSON()
}

// Name implements health.HealthMonitor.
func (lb *LoadBalancer) Name() string {
	return lb.Link
}

// Status implements health.HealthMonitor.
func (lb *LoadBalancer) Status() types.HealthStatus {
	status, _ := lb.status()
	return status
}

// Detail implements health.HealthMonitor.
func (lb *LoadBalancer) Detail() string {
	_, numHealthy := lb.status()
	return fmt.Sprintf("%d/%d servers are healthy", numHealthy, lb.pool.Size())
}

func (lb *LoadBalancer) status() (status types.HealthStatus, numHealthy int) {
	if lb.pool.Size() == 0 {
		return types.StatusUnknown, 0
	}

	// should be healthy if at least one server is healthy
	numHealthy = 0
	for _, srv := range lb.pool.Iter {
		if srv.Status().Good() {
			numHealthy++
		}
	}
	if numHealthy == 0 {
		return types.StatusUnhealthy, numHealthy
	}
	return types.StatusHealthy, numHealthy
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

func (lb *LoadBalancer) availServers() []types.LoadBalancerServer {
	avail := make([]types.LoadBalancerServer, 0, lb.pool.Size())
	for _, srv := range lb.pool.Iter {
		if srv.Status().Good() {
			avail = append(avail, srv)
		}
	}
	return avail
}

// isIdlewatcherRequest checks if this is an idlewatcher-related request
func isIdlewatcherRequest(r *http.Request) bool {
	// Check for explicit idlewatcher paths
	if r.URL.Path == idlewatcher.WakeEventsPath ||
		r.URL.Path == idlewatcher.FavIconPath ||
		r.URL.Path == idlewatcher.LoadingPageCSSPath ||
		r.URL.Path == idlewatcher.LoadingPageJSPath {
		return true
	}

	return false
}
