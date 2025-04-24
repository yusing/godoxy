package routes

import (
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/docker"
	"github.com/yusing/go-proxy/internal/homepage"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/pool"
	"github.com/yusing/go-proxy/internal/watcher/health"

	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
)

type (
	//nolint:interfacebloat // this is for avoiding circular imports
	Route interface {
		task.TaskStarter
		task.TaskFinisher
		pool.Object
		ProviderName() string
		TargetURL() *net.URL
		HealthMonitor() health.HealthMonitor
		Reference() string

		Started() bool

		IdlewatcherConfig() *idlewatcher.Config
		HealthCheckConfig() *health.HealthCheckConfig
		LoadBalanceConfig() *loadbalance.Config
		HomepageConfig() *homepage.ItemConfig
		HomepageItem() *homepage.Item
		ContainerInfo() *docker.Container

		Agent() *agent.AgentConfig

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
		net.Stream
	}
)
