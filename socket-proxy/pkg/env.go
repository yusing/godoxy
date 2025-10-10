package socketproxy

import "github.com/yusing/goutils/env"

var (
	DockerSocket,
	ListenAddr string

	DockerPost,
	DockerRestarts,
	DockerStart,
	DockerStop,
	DockerAuth,
	DockerBuild,
	DockerCommit,
	DockerConfigs,
	DockerContainers,
	DockerDistribution,
	DockerEvents,
	DockerExec,
	DockerGrpc,
	DockerImages,
	DockerInfo,
	DockerNetworks,
	DockerNodes,
	DockerPing,
	DockerPlugins,
	DockerSecrets,
	DockerServices,
	DockerSession,
	DockerSwarm,
	DockerSystem,
	DockerTasks,
	DockerVersion,
	DockerVolumes bool
)

func init() {
	Load()
}

func Load() {
	DockerSocket = env.GetEnvString("DOCKER_SOCKET", "/var/run/docker.sock")
	ListenAddr = env.GetEnvString("LISTEN_ADDR", env.GetEnvString("DOCKER_SOCKET_ADDR", "")) // default to disabled

	DockerPost = env.GetEnvBool("POST", false)
	DockerRestarts = env.GetEnvBool("ALLOW_RESTARTS", false)
	DockerStart = env.GetEnvBool("ALLOW_START", false)
	DockerStop = env.GetEnvBool("ALLOW_STOP", false)
	DockerAuth = env.GetEnvBool("AUTH", false)
	DockerBuild = env.GetEnvBool("BUILD", false)
	DockerCommit = env.GetEnvBool("COMMIT", false)
	DockerConfigs = env.GetEnvBool("CONFIGS", false)
	DockerContainers = env.GetEnvBool("CONTAINERS", false)
	DockerDistribution = env.GetEnvBool("DISTRIBUTION", false)
	DockerEvents = env.GetEnvBool("EVENTS", true)
	DockerExec = env.GetEnvBool("EXEC", false)
	DockerGrpc = env.GetEnvBool("GRPC", false)
	DockerImages = env.GetEnvBool("IMAGES", false)
	DockerInfo = env.GetEnvBool("INFO", false)
	DockerNetworks = env.GetEnvBool("NETWORKS", false)
	DockerNodes = env.GetEnvBool("NODES", false)
	DockerPing = env.GetEnvBool("PING", true)
	DockerPlugins = env.GetEnvBool("PLUGINS", false)
	DockerSecrets = env.GetEnvBool("SECRETS", false)
	DockerServices = env.GetEnvBool("SERVICES", false)
	DockerSession = env.GetEnvBool("SESSION", false)
	DockerSwarm = env.GetEnvBool("SWARM", false)
	DockerSystem = env.GetEnvBool("SYSTEM", false)
	DockerTasks = env.GetEnvBool("TASKS", false)
	DockerVersion = env.GetEnvBool("VERSION", true)
	DockerVolumes = env.GetEnvBool("VOLUMES", false)
}
