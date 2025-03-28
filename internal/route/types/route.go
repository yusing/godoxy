package types

import (
	"net/http"

	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/docker"
	idlewatcher "github.com/yusing/go-proxy/internal/docker/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/homepage"
	net "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/health"

	loadbalance "github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
)

type (
	//nolint:interfacebloat // this is for avoiding circular imports
	Route interface {
		task.TaskStarter
		task.TaskFinisher
		ProviderName() string
		TargetName() string
		TargetURL() *net.URL
		HealthMonitor() health.HealthMonitor

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
	StreamRoute interface {
		Route
		net.Stream
	}
)
