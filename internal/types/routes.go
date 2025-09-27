package types

import (
	"net/http"

	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/task"
	"github.com/yusing/godoxy/internal/utils/pool"
	"github.com/yusing/goutils/http/reverseproxy"
)

type (
	Route interface {
		task.TaskStarter
		task.TaskFinisher
		pool.Object
		ProviderName() string
		GetProvider() RouteProvider
		TargetURL() *nettypes.URL
		HealthMonitor() HealthMonitor
		SetHealthMonitor(m HealthMonitor)
		References() []string

		Started() <-chan struct{}

		IdlewatcherConfig() *IdlewatcherConfig
		HealthCheckConfig() *HealthCheckConfig
		LoadBalanceConfig() *LoadBalancerConfig
		HomepageItem() homepage.Item
		DisplayName() string
		ContainerInfo() *Container

		GetAgent() *agent.AgentConfig

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
	StreamRoute interface {
		Route
		nettypes.Stream
		Stream() nettypes.Stream
	}
	RouteProvider interface {
		GetRoute(alias string) (r Route, ok bool)
		IterRoutes(yield func(alias string, r Route) bool)
		FindService(project, service string) (r Route, ok bool)
		ShortName() string
	}
)
