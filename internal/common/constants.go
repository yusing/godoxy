package common

// file, folder structure

const (
	ConfigBasePath = "config"
	ConfigFileName = "config.yml"
	ConfigPath     = ConfigBasePath + "/" + ConfigFileName

	DataDir           = "data"
	IconListCachePath = DataDir + "/.icon_list_cache.json"

	NamespaceHomepageOverrides = ".homepage"

	MiddlewareComposeBasePath = ConfigBasePath + "/middlewares"

	ErrorPagesBasePath = "error_pages"
)

var RequiredDirectories = []string{
	ConfigBasePath,
	DataDir,
	ErrorPagesBasePath,
	MiddlewareComposeBasePath,
}

const DockerHostFromEnv = "$DOCKER_HOST"

const (
	WakeTimeoutDefault = "3m"
	StopTimeoutDefault = "3m"
	StopMethodDefault  = "stop"
)
