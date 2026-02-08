package idlewatcher

import "errors"

type ContainerStatus string

const (
	ContainerStatusError   ContainerStatus = "error"
	ContainerStatusRunning ContainerStatus = "running"
	ContainerStatusPaused  ContainerStatus = "paused"
	ContainerStatusStopped ContainerStatus = "stopped"
)

var ErrUnexpectedContainerStatus = errors.New("unexpected container status")
