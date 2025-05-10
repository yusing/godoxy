package env

import (
	"os"

	"github.com/yusing/go-proxy/internal/common"
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
	AgentSkipClientCertCheck bool
	AgentCACert              string
	AgentSSLCert             string
	DockerSocket             string
)

func init() {
	Load()
}

func Load() {
	DockerSocket = common.GetEnvString("DOCKER_SOCKET", "/var/run/docker.sock")
	AgentName = common.GetEnvString("AGENT_NAME", DefaultAgentName())
	AgentPort = common.GetEnvInt("AGENT_PORT", 8890)
	AgentSkipClientCertCheck = common.GetEnvBool("AGENT_SKIP_CLIENT_CERT_CHECK", false)

	AgentCACert = common.GetEnvString("AGENT_CA_CERT", "")
	AgentSSLCert = common.GetEnvString("AGENT_SSL_CERT", "")
}
