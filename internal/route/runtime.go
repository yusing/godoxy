package route

import (
	"errors"

	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/routing"
)

var ErrBuilderNotInitialized = errors.New("route builder not initialized")

type BuildFunc func(route *Route) (impl routing.Route, agent *agentpool.Agent, err error)
type HealthMonitorFunc func(route routing.Route) health.HealthMonitor

var build BuildFunc
var newHealthMonitor HealthMonitorFunc

func InitBuilder(fn BuildFunc) {
	build = fn
}

func InitHealthMonitor(fn HealthMonitorFunc) {
	newHealthMonitor = fn
}

// InitRuntime is kept as a temporary compatibility shim while the refactor
// updates callers to builder terminology.
func InitRuntime(fn BuildFunc) {
	InitBuilder(fn)
}
