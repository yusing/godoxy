package agent

type (
	ContainerRuntime string
	AgentEnvConfig   struct {
		Name             string
		Port             int
		StreamPort       int
		CACert           string
		SSLCert          string
		ContainerRuntime ContainerRuntime
	}
	AgentComposeConfig struct {
		Image string
		*AgentEnvConfig
	}
	Generator interface {
		Generate() (string, error)
	}
)

const (
	ContainerRuntimeDocker ContainerRuntime = "docker"
	ContainerRuntimePodman ContainerRuntime = "podman"
	// ContainerRuntimeNerdctl ContainerRuntime = "nerdctl"
)
