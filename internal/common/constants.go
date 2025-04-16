package common

import "time"

const DockerHostFromEnv = "$DOCKER_HOST"

const (
	HealthCheckIntervalDefault = 5 * time.Second
	HealthCheckTimeoutDefault  = 5 * time.Second
)
