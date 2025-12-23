package docker

import (
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/godoxy/internal/types"
)

var idDockerCfgMap = xsync.NewMap[string, types.DockerProviderConfig](xsync.WithPresize(100))

func GetDockerCfgByContainerID(id string) (types.DockerProviderConfig, bool) {
	return idDockerCfgMap.Load(id)
}

func SetDockerCfgByContainerID(id string, cfg types.DockerProviderConfig) {
	idDockerCfgMap.Store(id, cfg)
}

func DeleteDockerCfgByContainerID(id string) {
	idDockerCfgMap.Delete(id)
}
