package route

import (
	"net/http"

	"github.com/yusing/go-proxy/internal/api/v1/favicon"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/docker/idlewatcher"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	gphttp "github.com/yusing/go-proxy/internal/net/http"
	"github.com/yusing/go-proxy/internal/net/http/accesslog"
	"github.com/yusing/go-proxy/internal/net/http/loadbalancer"
	loadbalance "github.com/yusing/go-proxy/internal/net/http/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/net/http/middleware"
	metricslogger "github.com/yusing/go-proxy/internal/net/http/middleware/metrics_logger"
	"github.com/yusing/go-proxy/internal/net/http/reverseproxy"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

type (
	ReveseProxyRoute struct {
		*Route

		HealthMon health.HealthMonitor `json:"health,omitempty"`

		loadBalancer *loadbalancer.LoadBalancer
		handler      http.Handler
		rp           *reverseproxy.ReverseProxy

		task *task.Task
	}
)

// var globalMux    = http.NewServeMux() // TODO: support regex subdomain matching.

func NewReverseProxyRoute(base *Route) (*ReveseProxyRoute, gperr.Error) {
	httpConfig := base.HTTPConfig

	if httpConfig.NoTLSVerify {
		trans = gphttp.DefaultTransportNoTLS
	}
	if httpConfig.ResponseHeaderTimeout > 0 {
		trans = trans.Clone()
		trans.ResponseHeaderTimeout = httpConfig.ResponseHeaderTimeout
	}

	service := base.TargetName()
	rp := reverseproxy.NewReverseProxy(service, base.ProxyURL, trans)

	if len(base.Middlewares) > 0 {
		err := middleware.PatchReverseProxy(rp, base.Middlewares)
		if err != nil {
			return nil, err
		}
	}

	r := &ReveseProxyRoute{
		Route: base,
		rp:    rp,
	}
	return r, nil
}

func (r *ReveseProxyRoute) String() string {
	return r.TargetName()
}

// Start implements task.TaskStarter.
func (r *ReveseProxyRoute) Start(parent task.Parent) gperr.Error {
	r.task = parent.Subtask("http."+r.TargetName(), false)

	switch {
	case r.UseIdleWatcher():
		waker, err := idlewatcher.NewHTTPWaker(parent, r, r.rp)
		if err != nil {
			r.task.Finish(err)
			return err
		}
		r.handler = waker
		r.HealthMon = waker
	case r.UseHealthCheck():
		if r.IsDocker() {
			client, err := docker.ConnectClient(r.Idlewatcher.DockerHost)
			if err == nil {
				fallback := monitor.NewHTTPHealthChecker(r.rp.TargetURL, r.HealthCheck)
				r.HealthMon = monitor.NewDockerHealthMonitor(client, r.Idlewatcher.ContainerID, r.TargetName(), r.HealthCheck, fallback)
				r.task.OnCancel("close_docker_client", client.Close)
			}
		}
		if r.HealthMon == nil {
			r.HealthMon = monitor.NewHTTPHealthMonitor(r.rp.TargetURL, r.HealthCheck)
		}
	}

	if r.UseAccessLog() {
		var err error
		r.rp.AccessLogger, err = accesslog.NewFileAccessLogger(r.task, r.AccessLog)
		if err != nil {
			r.task.Finish(err)
			return gperr.Wrap(err)
		}
	}

	if r.handler == nil {
		pathPatterns := r.PathPatterns
		switch {
		case len(pathPatterns) == 0:
			r.handler = r.rp
		case len(pathPatterns) == 1 && pathPatterns[0] == "/":
			r.handler = r.rp
		default:
			logging.Warn().
				Str("route", r.TargetName()).
				Msg("`path_patterns` for reverse proxy is deprecated. Use `rules` instead.")
			mux := gphttp.NewServeMux()
			patErrs := gperr.NewBuilder("invalid path pattern(s)")
			for _, p := range pathPatterns {
				patErrs.Add(mux.HandleFunc(p, r.rp.HandlerFunc))
			}
			if err := patErrs.Error(); err != nil {
				r.task.Finish(err)
				return err
			}
			r.handler = mux
		}
	}

	if len(r.Rules) > 0 {
		r.handler = r.Rules.BuildHandler(r.TargetName(), r.handler)
	}

	if r.HealthMon != nil {
		if err := r.HealthMon.Start(r.task); err != nil {
			return err
		}
	}

	if common.PrometheusEnabled {
		metricsLogger := metricslogger.NewMetricsLogger(r.TargetName())
		r.handler = metricsLogger.GetHandler(r.handler)
		r.task.OnCancel("reset_metrics", metricsLogger.ResetMetrics)
	}

	if r.UseLoadBalance() {
		r.addToLoadBalancer(parent)
	} else {
		routes.SetHTTPRoute(r.TargetName(), r)
		r.task.OnCancel("entrypoint_remove_route", func() {
			routes.DeleteHTTPRoute(r.TargetName())
		})
	}

	r.task.OnCancel("reset_favicon", func() { favicon.PruneRouteIconCache(r) })
	return nil
}

// Task implements task.TaskStarter.
func (r *ReveseProxyRoute) Task() *task.Task {
	return r.task
}

// Finish implements task.TaskFinisher.
func (r *ReveseProxyRoute) Finish(reason any) {
	r.task.Finish(reason)
}

func (r *ReveseProxyRoute) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.handler.ServeHTTP(w, req)
}

func (r *ReveseProxyRoute) HealthMonitor() health.HealthMonitor {
	return r.HealthMon
}

func (r *ReveseProxyRoute) addToLoadBalancer(parent task.Parent) {
	var lb *loadbalancer.LoadBalancer
	cfg := r.LoadBalance
	l, ok := routes.GetHTTPRoute(cfg.Link)
	var linked *ReveseProxyRoute
	if ok {
		linked = l.(*ReveseProxyRoute)
		lb = linked.loadBalancer
		lb.UpdateConfigIfNeeded(cfg)
		if linked.Homepage.IsEmpty() && !r.Homepage.IsEmpty() {
			linked.Homepage = r.Homepage
		}
	} else {
		lb = loadbalancer.New(cfg)
		_ = lb.Start(parent) // always return nil
		linked = &ReveseProxyRoute{
			Route: &Route{
				Alias:    cfg.Link,
				Homepage: r.Homepage,
			},
			HealthMon:    lb,
			loadBalancer: lb,
			handler:      lb,
		}
		routes.SetHTTPRoute(cfg.Link, linked)
	}
	r.loadBalancer = lb

	server := loadbalance.NewServer(r.task.Name(), r.rp.TargetURL, r.LoadBalance.Weight, r.handler, r.HealthMon)
	lb.AddServer(server)
	r.task.OnCancel("lb_remove_server", func() {
		lb.RemoveServer(server)
	})
}
