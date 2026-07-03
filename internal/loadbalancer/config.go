package loadbalancer

import (
	"time"

	strutils "github.com/yusing/goutils/strings"
)

type Config struct {
	Link         string         `json:"link"`
	Mode         Mode           `json:"mode"`
	Weight       int            `json:"weight"`
	Sticky       bool           `json:"sticky"`
	StickyMaxAge time.Duration  `json:"sticky_max_age"`
	Options      map[string]any `json:"options,omitempty"`
} // @name LoadBalancerConfig

type Mode string // @name LoadBalancerMode

const (
	ModeUnset      Mode = ""
	ModeRoundRobin Mode = "roundrobin"
	ModeLeastConn  Mode = "leastconn"
	ModeIPHash     Mode = "iphash"
)

const StickyMaxAgeDefault = 1 * time.Hour

func (mode *Mode) ValidateUpdate() bool {
	switch strutils.ToLowerNoSnake(string(*mode)) {
	case "":
		return true
	case string(ModeRoundRobin):
		*mode = ModeRoundRobin
		return true
	case string(ModeLeastConn):
		*mode = ModeLeastConn
		return true
	case string(ModeIPHash):
		*mode = ModeIPHash
		return true
	}
	*mode = ModeRoundRobin
	return false
}
