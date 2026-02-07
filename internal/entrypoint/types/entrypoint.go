package entrypoint

import (
	"github.com/yusing/godoxy/internal/types"
)

type Entrypoint interface {
	SupportProxyProtocol() bool

	DisablePoolsLog(v bool)

	GetRoute(alias string) (types.Route, bool)
	AddRoute(r types.Route) error
	IterRoutes(yield func(r types.Route) bool)
	NumRoutes() int
	RoutesByProvider() map[string][]types.Route

	HTTPRoutes() PoolLike[types.HTTPRoute]
	StreamRoutes() PoolLike[types.StreamRoute]
	ExcludedRoutes() RWPoolLike[types.Route]

	GetHealthInfo() map[string]types.HealthInfo
	GetHealthInfoWithoutDetail() map[string]types.HealthInfoWithoutDetail
	GetHealthInfoSimple() map[string]types.HealthStatus
}

type PoolLike[Route types.Route] interface {
	Get(alias string) (Route, bool)
	Iter(yield func(alias string, r Route) bool)
	Size() int
}

type RWPoolLike[Route types.Route] interface {
	PoolLike[Route]
	Add(r Route)
	Del(r Route)
}
