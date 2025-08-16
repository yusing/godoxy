package types

import (
	"encoding/json"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	LabelMap = map[string]any

	PortMapping = map[int]container.Port
	Container   struct {
		_ utils.NoCopy

		DockerHost    string          `json:"docker_host"`
		Image         *ContainerImage `json:"image"`
		ContainerName string          `json:"container_name"`
		ContainerID   string          `json:"container_id"`

		Agent *agent.AgentConfig `json:"agent"`

		Labels            map[string]string  `json:"-"`
		IdlewatcherConfig *IdlewatcherConfig `json:"idlewatcher_config"`

		Mounts []string `json:"mounts"`

		Network            string      `json:"network,omitempty"`
		PublicPortMapping  PortMapping `json:"public_ports"`  // non-zero publicPort:types.Port
		PrivatePortMapping PortMapping `json:"private_ports"` // privatePort:types.Port
		PublicHostname     string      `json:"public_hostname"`
		PrivateHostname    string      `json:"private_hostname"`

		Aliases           []string `json:"aliases"`
		IsExcluded        bool     `json:"is_excluded"`
		IsExplicit        bool     `json:"is_explicit"`
		IsHostNetworkMode bool     `json:"is_host_network_mode"`
		Running           bool     `json:"running"`

		Errors *ContainerError `json:"errors" swaggertype:"string"`
	} // @name Container
	ContainerImage struct {
		Author string `json:"author,omitempty"`
		Name   string `json:"name"`
		Tag    string `json:"tag,omitempty"`
	} // @name ContainerImage

	ContainerError struct {
		errs *gperr.Builder
	}
)

func (e *ContainerError) Add(err error) {
	if e.errs == nil {
		e.errs = gperr.NewBuilder()
	}
	e.errs.Add(err)
}

func (e *ContainerError) Error() string {
	if e.errs == nil {
		return "<niL>"
	}
	return e.errs.String()
}

func (e *ContainerError) Unwrap() error {
	return e.errs.Error()
}

func (e *ContainerError) MarshalJSON() ([]byte, error) {
	err := e.errs.Error().(interface{ Plain() []byte })
	return json.Marshal(string(err.Plain()))
}
