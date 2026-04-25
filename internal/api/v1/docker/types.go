package dockerapi

import "github.com/docker/docker/api/types/container"

type (
	ContainerStartOptions   = container.StartOptions // @name ContainerStartOptions
	ContainerStopOptions    = container.StopOptions  // @name ContainerStopOptions
	ContainerRestartOptions = container.StopOptions  // @name ContainerRestartOptions

	ContainerListOptions = container.ListOptions // @name ContainerListOptions
	ContainerLogsOptions = container.LogsOptions // @name ContainerLogsOptions

	ContainerStatsResponse container.StatsResponse // @name ContainerStatsResponse
)
