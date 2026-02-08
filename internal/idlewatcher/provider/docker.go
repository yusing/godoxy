package provider

import (
	"context"
	"fmt"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/yusing/godoxy/internal/docker"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher"
)

type DockerProvider struct {
	client      *docker.SharedClient
	watcher     watcher.DockerWatcher
	containerID string
}

var startOptions = client.ContainerStartOptions{}

func NewDockerProvider(dockerCfg types.DockerProviderConfig, containerID string) (idlewatcher.Provider, error) {
	client, err := docker.NewClient(dockerCfg)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{
		client:      client,
		watcher:     watcher.NewDockerWatcher(dockerCfg),
		containerID: containerID,
	}, nil
}

func (p *DockerProvider) ContainerPause(ctx context.Context) error {
	_, err := p.client.ContainerPause(ctx, p.containerID, client.ContainerPauseOptions{})
	return err
}

func (p *DockerProvider) ContainerUnpause(ctx context.Context) error {
	_, err := p.client.ContainerUnpause(ctx, p.containerID, client.ContainerUnpauseOptions{})
	return err
}

func (p *DockerProvider) ContainerStart(ctx context.Context) error {
	_, err := p.client.ContainerStart(ctx, p.containerID, startOptions)
	return err
}

func (p *DockerProvider) ContainerStop(ctx context.Context, signal types.ContainerSignal, timeout int) error {
	_, err := p.client.ContainerStop(ctx, p.containerID, client.ContainerStopOptions{
		Signal:  string(signal),
		Timeout: &timeout,
	})
	return err
}

func (p *DockerProvider) ContainerKill(ctx context.Context, signal types.ContainerSignal) error {
	_, err := p.client.ContainerKill(ctx, p.containerID, client.ContainerKillOptions{
		Signal: string(signal),
	})
	return err
}

func (p *DockerProvider) ContainerStatus(ctx context.Context) (idlewatcher.ContainerStatus, error) {
	status, err := p.client.ContainerInspect(ctx, p.containerID, client.ContainerInspectOptions{})
	if err != nil {
		return idlewatcher.ContainerStatusError, err
	}
	switch status.Container.State.Status {
	case container.StateRunning:
		return idlewatcher.ContainerStatusRunning, nil
	case container.StateExited, container.StateDead, container.StateRestarting:
		return idlewatcher.ContainerStatusStopped, nil
	case container.StatePaused:
		return idlewatcher.ContainerStatusPaused, nil
	}
	return idlewatcher.ContainerStatusError, fmt.Errorf("%w: %s", idlewatcher.ErrUnexpectedContainerStatus, status.Container.State.Status)
}

func (p *DockerProvider) Watch(ctx context.Context) (eventCh <-chan watcher.Event, errCh <-chan error) {
	return p.watcher.EventsWithOptions(ctx, watcher.DockerListOptions{
		Filters: watcher.NewDockerFilters(
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
