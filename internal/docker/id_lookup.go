package docker

import (
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/godoxy/internal/types"
)

var containerConfigRegistry = xsync.NewMap[string, types.DockerProviderConfig](xsync.WithPresize(100))

func LookupContainerConfig(id string) (types.DockerProviderConfig, bool) {
	return containerConfigRegistry.Load(id)
}

func RegisterContainerConfig(id string, cfg types.DockerProviderConfig) {
	containerConfigRegistry.Store(id, cfg)
}

func UnregisterContainerConfig(id string) {
	containerConfigRegistry.Delete(id)
}
