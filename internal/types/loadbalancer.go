package types

import (
	"net/http"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	LoadBalancerConfig struct {
		Link    string           `json:"link"`
		Mode    LoadBalancerMode `json:"mode"`
		Weight  int              `json:"weight"`
		Options map[string]any   `json:"options,omitempty"`
	} // @name LoadBalancerConfig
	LoadBalancerMode   string // @name LoadBalancerMode
	LoadBalancerServer interface {
		http.Handler
		HealthMonitor
		Name() string
		Key() string
		URL() *nettypes.URL
		Weight() int
		SetWeight(weight int)
		TryWake() error
	}
	LoadBalancerServers []LoadBalancerServer
)

const (
	LoadbalanceModeUnset      LoadBalancerMode = ""
	LoadbalanceModeRoundRobin LoadBalancerMode = "roundrobin"
	LoadbalanceModeLeastConn  LoadBalancerMode = "leastconn"
	LoadbalanceModeIPHash     LoadBalancerMode = "iphash"
)

func (mode *LoadBalancerMode) ValidateUpdate() bool {
	switch strutils.ToLowerNoSnake(string(*mode)) {
	case "":
		return true
	case string(LoadbalanceModeRoundRobin):
		*mode = LoadbalanceModeRoundRobin
		return true
	case string(LoadbalanceModeLeastConn):
		*mode = LoadbalanceModeLeastConn
		return true
	case string(LoadbalanceModeIPHash):
		*mode = LoadbalanceModeIPHash
		return true
	}
	*mode = LoadbalanceModeRoundRobin
	return false
}
