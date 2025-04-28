package common

import (
	"time"
)

// file, folder structure

const (
	DotEnvPath        = ".env"
	DotEnvExamplePath = ".env.example"

	ConfigBasePath        = "config"
	ConfigFileName        = "config.yml"
	ConfigExampleFileName = "config.example.yml"
	ConfigPath            = ConfigBasePath + "/" + ConfigFileName

	IconListCachePath = ConfigBasePath + "/.icon_list_cache.json"

	NamespaceHomepageOverrides = ".homepage"
	NamespaceIconCache         = ".icon_cache"

	MiddlewareComposeBasePath = ConfigBasePath + "/middlewares"

	ComposeFileName        = "compose.yml"
	ComposeExampleFileName = "compose.example.yml"

	DataDir = "data"

	ErrorPagesBasePath = "error_pages"
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

	WakeTimeoutDefault = "3m"
	StopTimeoutDefault = "3m"
	StopMethodDefault  = "stop"
)
