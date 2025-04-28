package route

import (
	"crypto/tls"
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/agent/pkg/agentproxy"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/homepage"
	"github.com/yusing/go-proxy/internal/idlewatcher"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer"
	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/internal/watcher/health/monitor"
)

type ReveseProxyRoute struct {
	*Route

	HealthMon health.HealthMonitor `json:"health,omitempty"`

	loadBalancer *loadbalancer.LoadBalancer
	handler      http.Handler
	rp           *reverseproxy.ReverseProxy

	task *task.Task
}

var _ routes.ReverseProxyRoute = (*ReveseProxyRoute)(nil)

// var globalMux    = http.NewServeMux() // TODO: support regex subdomain matching.

func NewReverseProxyRoute(base *Route) (*ReveseProxyRoute, gperr.Error) {
	httpConfig := base.HTTPConfig
	proxyURL := base.ProxyURL

	var trans *http.Transport
	a := base.Agent()
	if a != nil {
		trans = a.Transport()
		proxyURL = types.NewURL(agent.HTTPProxyURL)
	} else {
		trans = gphttp.NewTransport()
		if httpConfig.NoTLSVerify {
			trans.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		if httpConfig.ResponseHeaderTimeout > 0 {
			trans.ResponseHeaderTimeout = httpConfig.ResponseHeaderTimeout
		}
	}

	service := base.Name()
	rp := reverseproxy.NewReverseProxy(service, proxyURL, trans)

	if len(base.Middlewares) > 0 {
		err := middleware.PatchReverseProxy(rp, base.Middlewares)
		if err != nil {
			return nil, err
		}
	}

	if a != nil {
		headers := &agentproxy.AgentProxyHeaders{
			Host:                  base.ProxyURL.Host,
			IsHTTPS:               base.ProxyURL.Scheme == "https",
			SkipTLSVerify:         httpConfig.NoTLSVerify,
			ResponseHeaderTimeout: int(httpConfig.ResponseHeaderTimeout.Seconds()),
		}
		ori := rp.HandlerFunc
		rp.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			agentproxy.SetAgentProxyHeaders(r, headers)
			ori(w, r)
		}
	}

	r := &ReveseProxyRoute{
		Route: base,
		rp:    rp,
	}
	return r, nil
}

// ReverseProxy implements routes.ReverseProxyRoute.
func (r *ReveseProxyRoute) ReverseProxy() *reverseproxy.ReverseProxy {
	return r.rp
}

// Start implements task.TaskStarter.
func (r *ReveseProxyRoute) Start(parent task.Parent) gperr.Error {
	if existing, ok := routes.HTTP.Get(r.Key()); ok && !r.UseLoadBalance() {
		return gperr.Errorf("route already exists: from provider %s and %s", existing.ProviderName(), r.ProviderName())
	}
	r.task = parent.Subtask("http."+r.Name(), false)

	switch {
	case r.UseIdleWatcher():
		waker, err := idlewatcher.NewWatcher(parent, r)
		if err != nil {
			r.task.Finish(err)
			return gperr.Wrap(err)
		}
		r.handler = waker
		r.HealthMon = waker
	case r.UseHealthCheck():
		r.HealthMon = monitor.NewMonitor(r)
	}

	if r.handler == nil {
		r.handler = r.rp
	}

	if r.UseAccessLog() {
		var err error
		r.rp.AccessLogger, err = accesslog.NewAccessLogger(r.task, r.AccessLog)
		if err != nil {
			r.task.Finish(err)
			return gperr.Wrap(err)
		}
	}

	if len(r.Rules) > 0 {
		r.handler = r.Rules.BuildHandler(r.Name(), r.handler)
	}

	if r.HealthMon != nil {
		if err := r.HealthMon.Start(r.task); err != nil {
			return err
		}
	}

	if r.UseLoadBalance() {
		r.addToLoadBalancer(parent)
	} else {
		routes.HTTP.Add(r)
		r.task.OnFinished("entrypoint_remove_route", func() {
			routes.HTTP.Del(r)
		})
	}

	r.task.OnCancel("reset_favicon", func() { homepage.PruneRouteIconCache(r) })
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
	l, ok := routes.HTTP.Get(cfg.Link)
	var linked *ReveseProxyRoute
	if ok {
		linked = l.(*ReveseProxyRoute)
		lb = linked.loadBalancer
		lb.UpdateConfigIfNeeded(cfg)
		if linked.Homepage == nil {
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
		routes.HTTP.Add(linked)
		r.task.OnFinished("entrypoint_remove_route", func() {
			routes.HTTP.Del(linked)
		})
	}
	r.loadBalancer = lb

	server := loadbalance.NewServer(r.task.Name(), r.ProxyURL, r.LoadBalance.Weight, r.handler, r.HealthMon)
	lb.AddServer(server)
	r.task.OnCancel("lb_remove_server", func() {
		lb.RemoveServer(server)
	})
}
