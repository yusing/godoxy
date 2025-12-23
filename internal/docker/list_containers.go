package docker

import (
	"context"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/yusing/godoxy/internal/types"
)

var listOptions = client.ContainerListOptions{
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

func ListContainers(ctx context.Context, dockerCfg types.DockerProviderConfig) ([]container.Summary, error) {
	dockerClient, err := NewClient(dockerCfg)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	containers, err := dockerClient.ContainerList(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	return containers.Items, nil
}

func IsErrConnectionFailed(err error) bool {
	return client.IsErrConnectionFailed(err)
}
