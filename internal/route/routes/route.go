package routes

import (
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/homepage"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/pool"
	"github.com/yusing/go-proxy/internal/watcher/health"

	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	nettypes "github.com/yusing/go-proxy/internal/net/types"
)

type (
	//nolint:interfacebloat // this is for avoiding circular imports
	Route interface {
		task.TaskStarter
		task.TaskFinisher
		pool.Object
		ProviderName() string
		GetProvider() Provider
		TargetURL() *nettypes.URL
		HealthMonitor() health.HealthMonitor
		SetHealthMonitor(m health.HealthMonitor)
		References() []string

		Started() <-chan struct{}

		IdlewatcherConfig() *idlewatcher.Config
		HealthCheckConfig() *health.HealthCheckConfig
		LoadBalanceConfig() *loadbalance.Config
		HomepageConfig() *homepage.ItemConfig
		HomepageItem() *homepage.Item
		ContainerInfo() *docker.Container

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
	Provider interface {
		GetRoute(alias string) (r Route, ok bool)
		IterRoutes(yield func(alias string, r Route) bool)
		FindService(project, service string) (r Route, ok bool)
		ShortName() string
	}
)
