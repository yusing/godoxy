package loadbalancer

import (
	"github.com/yusing/go-proxy/internal/net/gphttp/loadbalancer/types"
)

type (
	Server  = types.Server
	Servers = []types.Server
	Pool    = types.Pool
	Weight  = types.Weight
	Config  = types.Config
	Mode    = types.Mode
)
