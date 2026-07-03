package types

import (
	"net/http"

	"github.com/yusing/godoxy/internal/health"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

type (
	LoadBalancerServer interface {
		http.Handler
		health.HealthMonitor
		Name() string
		Key() string
		URL() *nettypes.URL
		Weight() int
		SetWeight(weight int)
		TryWake() error
	}
	LoadBalancerServers []LoadBalancerServer
)
