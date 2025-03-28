package docker

import (
	"context"
	"errors"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var listOptions = container.ListOptions{
	// created|restarting|running|removing|paused|exited|dead
	// Filters: filters.NewArgs(
	// 	filters.Arg("status", "created"),
	// 	filters.Arg("status", "restarting"),
	// 	filters.Arg("status", "running"),
	// 	filters.Arg("status", "paused"),
	// 	filters.Arg("status", "exited"),
	// ),
	All: true,
}

func ListContainers(clientHost string) ([]container.Summary, error) {
	dockerClient, err := NewClient(clientHost)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	ctx, cancel := context.WithTimeoutCause(context.Background(), 3*time.Second, errors.New("list containers timeout"))
	defer cancel()

	containers, err := dockerClient.ContainerList(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func IsErrConnectionFailed(err error) bool {
	return client.IsErrConnectionFailed(err)
}
