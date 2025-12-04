package common

// file, folder structure

const (
	DotEnvPath        = ".env"
	DotEnvExamplePath = ".env.example"

	ConfigBasePath        = "config"
	ConfigFileName        = "config.yml"
	ConfigExampleFileName = "config.example.yml"
	ConfigPath            = ConfigBasePath + "/" + ConfigFileName

	DataDir           = "data"
	IconListCachePath = DataDir + "/.icon_list_cache.json"

	NamespaceHomepageOverrides = ".homepage"
	NamespaceIconCache         = ".icon_cache"

	MiddlewareComposeBasePath = ConfigBasePath + "/middlewares"

	ComposeFileName        = "compose.yml"
	ComposeExampleFileName = "compose.example.yml"
	ErrorPagesBasePath     = "error_pages"
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
