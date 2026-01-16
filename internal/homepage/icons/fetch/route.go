package iconfetch

import (
	"net/http"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/pool"
)

type route interface {
	pool.Object
	ProviderName() string
	References() []string
	TargetURL() *nettypes.URL
	HealthMonitor() types.HealthMonitor
}

type httpRoute interface {
	route
	http.Handler
}
