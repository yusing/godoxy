package idlewatcher

import "github.com/yusing/godoxy/internal/gperr"

type ContainerStatus string

const (
	ContainerStatusError   ContainerStatus = "error"
	ContainerStatusRunning ContainerStatus = "running"
	ContainerStatusPaused  ContainerStatus = "paused"
	ContainerStatusStopped ContainerStatus = "stopped"
)

var ErrUnexpectedContainerStatus = gperr.New("unexpected container status")
