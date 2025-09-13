package docker

import (
	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
)

var idDockerHostMap = xsync.NewMap[string, string](xsync.WithPresize(100))

func GetDockerHostByContainerID(id string) (string, bool) {
	return idDockerHostMap.Load(id)
}

func SetDockerHostByContainerID(id, host string) {
	log.Debug().Str("id", id).Str("host", host).Int("size", idDockerHostMap.Size()).Msg("setting docker host by container id")
	idDockerHostMap.Store(id, host)
}

func DeleteDockerHostByContainerID(id string) {
	idDockerHostMap.Delete(id)
}
