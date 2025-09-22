package provider

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/gperr"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher"
)

type DockerProvider struct {
	client      *docker.SharedClient
	watcher     watcher.DockerWatcher
	containerID string
}

var startOptions = container.StartOptions{}

func NewDockerProvider(dockerHost, containerID string) (idlewatcher.Provider, error) {
	client, err := docker.NewClient(dockerHost)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{
		client:      client,
		watcher:     watcher.NewDockerWatcher(dockerHost),
		containerID: containerID,
	}, nil
}

func (p *DockerProvider) ContainerPause(ctx context.Context) error {
	return p.client.ContainerPause(ctx, p.containerID)
}

func (p *DockerProvider) ContainerUnpause(ctx context.Context) error {
	return p.client.ContainerUnpause(ctx, p.containerID)
}

func (p *DockerProvider) ContainerStart(ctx context.Context) error {
	return p.client.ContainerStart(ctx, p.containerID, startOptions)
}

func (p *DockerProvider) ContainerStop(ctx context.Context, signal types.ContainerSignal, timeout int) error {
	return p.client.ContainerStop(ctx, p.containerID, container.StopOptions{
		Signal:  string(signal),
		Timeout: &timeout,
	})
}

func (p *DockerProvider) ContainerKill(ctx context.Context, signal types.ContainerSignal) error {
	return p.client.ContainerKill(ctx, p.containerID, string(signal))
}

func (p *DockerProvider) ContainerStatus(ctx context.Context) (idlewatcher.ContainerStatus, error) {
	status, err := p.client.ContainerInspect(ctx, p.containerID)
	if err != nil {
		return idlewatcher.ContainerStatusError, err
	}
	switch status.State.Status {
	case "running":
		return idlewatcher.ContainerStatusRunning, nil
	case "exited", "dead", "restarting":
		return idlewatcher.ContainerStatusStopped, nil
	case "paused":
		return idlewatcher.ContainerStatusPaused, nil
	}
	return idlewatcher.ContainerStatusError, idlewatcher.ErrUnexpectedContainerStatus.Subject(status.State.Status)
}

func (p *DockerProvider) Watch(ctx context.Context) (eventCh <-chan watcher.Event, errCh <-chan gperr.Error) {
	return p.watcher.EventsWithOptions(ctx, watcher.DockerListOptions{
		Filters: watcher.NewDockerFilter(
			watcher.DockerFilterContainer,
			watcher.DockerFilterContainerNameID(p.containerID),
			watcher.DockerFilterStart,
			watcher.DockerFilterStop,
			watcher.DockerFilterDie,
			watcher.DockerFilterKill,
			watcher.DockerFilterDestroy,
			watcher.DockerFilterPause,
			watcher.DockerFilterUnpause,
		),
	})
}

func (p *DockerProvider) Close() {
	p.client.Close()
}
