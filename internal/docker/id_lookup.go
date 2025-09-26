package docker

import (
	"github.com/puzpuzpuz/xsync/v4"
)

var idDockerHostMap = xsync.NewMap[string, string](xsync.WithPresize(100))

func GetDockerHostByContainerID(id string) (string, bool) {
	return idDockerHostMap.Load(id)
}

func SetDockerHostByContainerID(id, host string) {
	idDockerHostMap.Store(id, host)
}

func DeleteDockerHostByContainerID(id string) {
	idDockerHostMap.Delete(id)
}
