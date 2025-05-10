package socketproxy

import (
	"log"
	"os"
	"strconv"
)

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

func GetEnv[T any](key string, defaultValue T, parser func(string) (T, error)) T {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return defaultValue
	}
	parsed, err := parser(value)
	if err != nil {
		log.Fatalf("env %s: invalid %T value: %s", key, parsed, value)
	}
	return parsed
}

func GetEnvString(key string, defaultValue string) string {
	return GetEnv(key, defaultValue, stringstring)
}

func GetEnvBool(key string, defaultValue bool) bool {
	return GetEnv(key, defaultValue, strconv.ParseBool)
}

func stringstring(s string) (string, error) {
	return s, nil
}

func Load() {
	DockerSocket = GetEnvString("DOCKER_SOCKET", "/var/run/docker.sock")
	ListenAddr = GetEnvString("LISTEN_ADDR", GetEnvString("DOCKER_SOCKET_ADDR", "")) // default to disabled

	DockerPost = GetEnvBool("POST", false)
	DockerRestarts = GetEnvBool("ALLOW_RESTARTS", false)
	DockerStart = GetEnvBool("ALLOW_START", false)
	DockerStop = GetEnvBool("ALLOW_STOP", false)
	DockerAuth = GetEnvBool("AUTH", false)
	DockerBuild = GetEnvBool("BUILD", false)
	DockerCommit = GetEnvBool("COMMIT", false)
	DockerConfigs = GetEnvBool("CONFIGS", false)
	DockerContainers = GetEnvBool("CONTAINERS", false)
	DockerDistribution = GetEnvBool("DISTRIBUTION", false)
	DockerEvents = GetEnvBool("EVENTS", true)
	DockerExec = GetEnvBool("EXEC", false)
	DockerGrpc = GetEnvBool("GRPC", false)
	DockerImages = GetEnvBool("IMAGES", false)
	DockerInfo = GetEnvBool("INFO", false)
	DockerNetworks = GetEnvBool("NETWORKS", false)
	DockerNodes = GetEnvBool("NODES", false)
	DockerPing = GetEnvBool("PING", true)
	DockerPlugins = GetEnvBool("PLUGINS", false)
	DockerSecrets = GetEnvBool("SECRETS", false)
	DockerServices = GetEnvBool("SERVICES", false)
	DockerSession = GetEnvBool("SESSION", false)
	DockerSwarm = GetEnvBool("SWARM", false)
	DockerSystem = GetEnvBool("SYSTEM", false)
	DockerTasks = GetEnvBool("TASKS", false)
	DockerVersion = GetEnvBool("VERSION", true)
	DockerVolumes = GetEnvBool("VOLUMES", false)
}
