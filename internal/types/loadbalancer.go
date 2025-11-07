package types

import (
	"net/http"
	"time"

	nettypes "github.com/yusing/godoxy/internal/net/types"
	strutils "github.com/yusing/goutils/strings"
)

type (
	LoadBalancerConfig struct {
		Link         string           `json:"link"`
		Mode         LoadBalancerMode `json:"mode"`
		Weight       int              `json:"weight"`
		Sticky       bool             `json:"sticky"`
		StickyMaxAge time.Duration    `json:"sticky_max_age"`
		Options      map[string]any   `json:"options,omitempty"`
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

const StickyMaxAgeDefault = 1 * time.Hour

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
