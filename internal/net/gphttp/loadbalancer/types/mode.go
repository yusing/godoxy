package types

import (
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type Mode string

const (
	ModeUnset      Mode = ""
	ModeRoundRobin Mode = "roundrobin"
	ModeLeastConn  Mode = "leastconn"
	ModeIPHash     Mode = "iphash"
)

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
