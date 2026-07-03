package routing

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/homepage"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/loadbalancer"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	basetypes "github.com/yusing/godoxy/internal/types"
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
		GetProvider() Provider
		ListenURL() *nettypes.URL
		TargetURL() *nettypes.URL
		HealthMonitor() health.HealthMonitor
		SetHealthMonitor(m health.HealthMonitor)
		References() []string
		ShouldExclude() bool

		Started() <-chan struct{}

		IdlewatcherConfig() *idlewatcher.Config
		HealthCheckConfig() health.HealthCheckConfig
		LoadBalanceConfig() *loadbalancer.Config
		HomepageItem() homepage.Item
		DisplayName() string
		ContainerInfo() *docker.Container
		InboundMTLSProfileRef() string
		RouteMiddlewares() map[string]basetypes.LabelMap

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
	Provider interface {
		Start(parent task.Parent) error
		LoadRoutes() error
		GetRoute(alias string) (r Route, ok bool)
		IterRoutes(yield func(alias string, r Route) bool)
		NumRoutes() int
		FindService(project, service string) (r Route, ok bool)
		Statistics() ProviderStats
		GetType() ProviderType
		ShortName() string
		String() string
	}
	Entrypoint interface {
		SupportProxyProtocol() bool
		DisablePoolsLog(v bool)
		GetRoute(alias string) (Route, bool)
		StartAddRoute(r Route) error
		IterRoutes(yield func(r Route) bool)
		NumRoutes() int
		RoutesByProvider() map[string][]Route
		HTTPRoutes() PoolLike[HTTPRoute]
		StreamRoutes() PoolLike[StreamRoute]
		ExcludedRoutes() RWPoolLike[Route]
		GetHealthInfo() map[string]health.HealthInfo
		GetHealthInfoWithoutDetail() map[string]health.HealthInfoWithoutDetail
		GetHealthInfoSimple() map[string]health.HealthStatus
	}
	PoolLike[T Route] interface {
		Get(alias string) (T, bool)
		Iter(yield func(alias string, r T) bool)
		Size() int
	}
	RWPoolLike[T Route] interface {
		PoolLike[T]
		Add(r T)
		Del(r T)
	}
)

type EntrypointContextKey struct{}

func SetEntrypointCtx(ctx interface{ SetValue(key any, value any) }, ep Entrypoint) {
	ctx.SetValue(EntrypointContextKey{}, ep)
}

func EntrypointFromCtx(ctx context.Context) Entrypoint {
	if ep, ok := ctx.Value(EntrypointContextKey{}).(Entrypoint); ok {
		return ep
	}
	return nil
}
