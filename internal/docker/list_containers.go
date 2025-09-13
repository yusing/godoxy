package docker

import (
	"context"

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

func ListContainers(ctx context.Context, clientHost string) ([]container.Summary, error) {
	dockerClient, err := NewClient(clientHost)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	containers, err := dockerClient.ContainerList(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func IsErrConnectionFailed(err error) bool {
	return client.IsErrConnectionFailed(err)
}
