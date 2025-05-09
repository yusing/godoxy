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

	DockerSocketAddr   string
	DockerPost         bool
	DockerRestarts     bool
	DockerStart        bool
	DockerStop         bool
	DockerAuth         bool
	DockerBuild        bool
	DockerCommit       bool
	DockerConfigs      bool
	DockerContainers   bool
	DockerDistribution bool
	DockerEvents       bool
	DockerExec         bool
	DockerGrpc         bool
	DockerImages       bool
	DockerInfo         bool
	DockerNetworks     bool
	DockerNodes        bool
	DockerPing         bool
	DockerPlugins      bool
	DockerSecrets      bool
	DockerServices     bool
	DockerSession      bool
	DockerSwarm        bool
	DockerSystem       bool
	DockerTasks        bool
	DockerVersion      bool
	DockerVolumes      bool
)

func init() {
	Load()
}

func Load() {
	AgentName = common.GetEnvString("AGENT_NAME", DefaultAgentName())
	AgentPort = common.GetEnvInt("AGENT_PORT", 8890)
	AgentSkipClientCertCheck = common.GetEnvBool("AGENT_SKIP_CLIENT_CERT_CHECK", false)

	AgentCACert = common.GetEnvString("AGENT_CA_CERT", "")
	AgentSSLCert = common.GetEnvString("AGENT_SSL_CERT", "")

	// docker socket proxy
	DockerSocketAddr = common.GetEnvString("DOCKER_SOCKET_ADDR", "127.0.0.1:2375")

	DockerPost = common.GetEnvBool("POST", false)
	DockerRestarts = common.GetEnvBool("ALLOW_RESTARTS", false)
	DockerStart = common.GetEnvBool("ALLOW_START", false)
	DockerStop = common.GetEnvBool("ALLOW_STOP", false)
	DockerAuth = common.GetEnvBool("AUTH", false)
	DockerBuild = common.GetEnvBool("BUILD", false)
	DockerCommit = common.GetEnvBool("COMMIT", false)
	DockerConfigs = common.GetEnvBool("CONFIGS", false)
	DockerContainers = common.GetEnvBool("CONTAINERS", false)
	DockerDistribution = common.GetEnvBool("DISTRIBUTION", false)
	DockerEvents = common.GetEnvBool("EVENTS", true)
	DockerExec = common.GetEnvBool("EXEC", false)
	DockerGrpc = common.GetEnvBool("GRPC", false)
	DockerImages = common.GetEnvBool("IMAGES", false)
	DockerInfo = common.GetEnvBool("INFO", false)
	DockerNetworks = common.GetEnvBool("NETWORKS", false)
	DockerNodes = common.GetEnvBool("NODES", false)
	DockerPing = common.GetEnvBool("PING", true)
	DockerPlugins = common.GetEnvBool("PLUGINS", false)
	DockerSecrets = common.GetEnvBool("SECRETS", false)
	DockerServices = common.GetEnvBool("SERVICES", false)
	DockerSession = common.GetEnvBool("SESSION", false)
	DockerSwarm = common.GetEnvBool("SWARM", false)
	DockerSystem = common.GetEnvBool("SYSTEM", false)
	DockerTasks = common.GetEnvBool("TASKS", false)
	DockerVersion = common.GetEnvBool("VERSION", true)
	DockerVolumes = common.GetEnvBool("VOLUMES", false)
}
