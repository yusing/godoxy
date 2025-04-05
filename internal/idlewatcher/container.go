package idlewatcher

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types/container"
)

type (
	containerMeta struct {
		ContainerID, ContainerName string
	}
	containerState struct {
		running bool
		ready   bool
		err     error
	}
)

func (w *Watcher) ContainerID() string {
	return w.route.ContainerInfo().ContainerID
}

func (w *Watcher) ContainerName() string {
	return w.route.ContainerInfo().ContainerName
}

func (w *Watcher) containerStop(ctx context.Context) error {
	return w.client.ContainerStop(ctx, w.ContainerID(), container.StopOptions{
		Signal:  string(w.Config().StopSignal),
		Timeout: &w.Config().StopTimeout,
	})
}

func (w *Watcher) containerPause(ctx context.Context) error {
	return w.client.ContainerPause(ctx, w.ContainerID())
}

func (w *Watcher) containerKill(ctx context.Context) error {
	return w.client.ContainerKill(ctx, w.ContainerID(), string(w.Config().StopSignal))
}

func (w *Watcher) containerUnpause(ctx context.Context) error {
	return w.client.ContainerUnpause(ctx, w.ContainerID())
}

func (w *Watcher) containerStart(ctx context.Context) error {
	return w.client.ContainerStart(ctx, w.ContainerID(), container.StartOptions{})
}

func (w *Watcher) containerStatus() (string, error) {
	ctx, cancel := context.WithTimeoutCause(w.task.Context(), dockerReqTimeout, errors.New("docker request timeout"))
	defer cancel()
	json, err := w.client.ContainerInspect(ctx, w.ContainerID())
	if err != nil {
		return "", err
	}
	return json.State.Status, nil
}
