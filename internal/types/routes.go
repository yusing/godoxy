package types

import (
	"net/http"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	provider "github.com/yusing/godoxy/internal/route/provider/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/http/reverseproxy"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

type (
	Route interface {
		task.TaskStarter
		task.TaskFinisher
		pool.Object
		zerolog.LogObjectMarshaler

		ProviderName() string
		GetProvider() RouteProvider
		ListenURL() *nettypes.URL
		TargetURL() *nettypes.URL
		HealthMonitor() HealthMonitor
		SetHealthMonitor(m HealthMonitor)
		References() []string
		ShouldExclude() bool

		Started() <-chan struct{}

		IdlewatcherConfig() *IdlewatcherConfig
		HealthCheckConfig() HealthCheckConfig
		LoadBalanceConfig() *LoadBalancerConfig
		HomepageItem() homepage.Item
		DisplayName() string
		ContainerInfo() *Container

		GetAgent() *agentpool.Agent

		IsDocker() bool
		IsAgent() bool
		UseLoadBalance() bool
		UseIdleWatcher() bool
		UseHealthCheck() bool
		UseAccessLog() bool
	}
	HTTPRoute interface {
		Route
		http.Handler
	}
	ReverseProxyRoute interface {
		HTTPRoute
		ReverseProxy() *reverseproxy.ReverseProxy
	}
	FileServerRoute interface {
		HTTPRoute
		RootPath() string
	}
	StreamRoute interface {
		Route
		nettypes.Stream
		Stream() nettypes.Stream
	}
	RouteProvider interface {
		Start(task.Parent) gperr.Error
		LoadRoutes() gperr.Error
		GetRoute(alias string) (r Route, ok bool)
		// should be used like `for _, r := range p.IterRoutes` (no braces), not calling it directly
		IterRoutes(yield func(alias string, r Route) bool)
		NumRoutes() int
		FindService(project, service string) (r Route, ok bool)
		Statistics() ProviderStats
		GetType() provider.Type
		ShortName() string
		String() string
	}
)
