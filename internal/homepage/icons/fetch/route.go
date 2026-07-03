package iconfetch

import (
	"net/http"

	"github.com/yusing/godoxy/internal/health"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/goutils/pool"
)

type route interface {
	pool.Object
	ProviderName() string
	References() []string
	TargetURL() *nettypes.URL
	HealthMonitor() health.HealthMonitor
}

type httpRoute interface {
	route
	http.Handler
}
