package env

import (
	"os"

	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/goutils/env"

	"github.com/rs/zerolog/log"
)

func DefaultAgentName() string {
	name, err := os.Hostname()
	if err != nil {
		return "agent"
	}
	return name
}

var (
	AgentName                string
	AgentPort                int
	AgentStreamPort          int
	AgentSkipClientCertCheck bool
	AgentCACert              string
	AgentSSLCert             string
	DockerSocket             string
	Runtime                  agent.ContainerRuntime
)

func init() {
	Load()
}

func Load() {
	DockerSocket = env.GetEnvString("DOCKER_SOCKET", "/var/run/docker.sock")
	AgentName = env.GetEnvString("AGENT_NAME", DefaultAgentName())
	AgentPort = env.GetEnvInt("AGENT_PORT", 8890)
	AgentStreamPort = env.GetEnvInt("AGENT_STREAM_PORT", AgentPort+1)
	AgentSkipClientCertCheck = env.GetEnvBool("AGENT_SKIP_CLIENT_CERT_CHECK", false)

	AgentCACert = env.GetEnvString("AGENT_CA_CERT", "")
	AgentSSLCert = env.GetEnvString("AGENT_SSL_CERT", "")
	Runtime = agent.ContainerRuntime(env.GetEnvString("RUNTIME", "docker"))

	switch Runtime {
	case agent.ContainerRuntimeDocker, agent.ContainerRuntimePodman: //, agent.ContainerRuntimeNerdctl:
	default:
		log.Fatal().Str("runtime", string(Runtime)).Msg("invalid runtime")
	}
}
