package common

import (
	"time"
)

// file, folder structure

const (
	DotEnvPath        = ".env"
	DotEnvExamplePath = ".env.example"

	ConfigBasePath         = "config"
	ConfigFileName         = "config.yml"
	ConfigExampleFileName  = "config.example.yml"
	ConfigPath             = ConfigBasePath + "/" + ConfigFileName
	HomepageJSONConfigPath = ConfigBasePath + "/.homepage.json"
	IconListCachePath      = ConfigBasePath + "/.icon_list_cache.json"
	IconCachePath          = ConfigBasePath + "/.icon_cache.json"

	MiddlewareComposeBasePath = ConfigBasePath + "/middlewares"

	ComposeFileName        = "compose.yml"
	ComposeExampleFileName = "compose.example.yml"

	ErrorPagesBasePath = "error_pages"

	AgentCertsBasePath = "certs"
)

var RequiredDirectories = []string{
	ConfigBasePath,
	ErrorPagesBasePath,
	MiddlewareComposeBasePath,
}

const DockerHostFromEnv = "$DOCKER_HOST"

const (
	HealthCheckIntervalDefault = 5 * time.Second
	HealthCheckTimeoutDefault  = 5 * time.Second

	WakeTimeoutDefault = "30s"
	StopTimeoutDefault = "30s"
	StopMethodDefault  = "stop"
)
