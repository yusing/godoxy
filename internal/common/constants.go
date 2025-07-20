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
	HealthCheckIntervalDefault        = 5 * time.Second
	HealthCheckTimeoutDefault         = 5 * time.Second
	HealthCheckDownNotifyDelayDefault = 15 * time.Second

	WakeTimeoutDefault = "3m"
	StopTimeoutDefault = "3m"
	StopMethodDefault  = "stop"
)
