package docker

import (
	"github.com/docker/docker/api/types/container"
	"github.com/yusing/ds/ordered"
	"github.com/yusing/godoxy/internal/agentpool"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	strutils "github.com/yusing/goutils/strings"
)

type (
	PortMapping = map[int]container.Port
	Container   struct {
		DockerCfg     types.DockerProviderConfig `json:"docker_cfg"`
		Image         *Image                     `json:"image"`
		ContainerName string                     `json:"container_name"`
		ContainerID   string                     `json:"container_id"`

		State container.ContainerState `json:"state"`

		Agent *agentpool.Agent `json:"agent"`

		Labels            map[string]string              `json:"-"`
		ActualLabels      map[string]string              `json:"labels"`
		IdlewatcherConfig *idlewatcher.IdlewatcherConfig `json:"idlewatcher_config"`

		Mounts *ordered.Map[string, string] `json:"mounts,omitempty" swaggertype:"object,string"`

		Network            string      `json:"network,omitempty"`
		PublicPortMapping  PortMapping `json:"public_ports"`
		PrivatePortMapping PortMapping `json:"private_ports"`
		PublicHostname     string      `json:"public_hostname"`
		PrivateHostname    string      `json:"private_hostname"`

		Aliases            []string `json:"aliases"`
		IsExcluded         bool     `json:"is_excluded"`
		IsExplicit         bool     `json:"is_explicit"`
		IsHostNetworkMode  bool     `json:"is_host_network_mode"`
		HealthCheckEnabled bool     `json:"-"`
		Running            bool     `json:"running"`

		Errors *ContainerError `json:"errors" swaggertype:"string"`
	} // @name Container
	Image struct {
		Author  string `json:"author,omitempty"`
		Name    string `json:"name"`
		Tag     string `json:"tag,omitempty"`
		SHA256  string `json:"sha256,omitempty"`
		Version string `json:"version,omitempty"`
	} // @name ContainerImage

	ContainerError struct {
		errs gperr.Builder
	}
)

func (e *ContainerError) Add(err error) {
	e.errs.Add(err)
}

func (e *ContainerError) Error() string {
	return e.errs.String()
}

func (e *ContainerError) Unwrap() error {
	return e.errs.Error()
}

func (e *ContainerError) MarshalJSON() ([]byte, error) {
	err := e.errs.Error().(gperr.PlainError)
	return strutils.MarshalJSON(string(err.Plain()))
}
