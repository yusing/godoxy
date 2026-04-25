package docker

import (
	"context"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/yusing/godoxy/internal/types"
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

func ListContainers(ctx context.Context, dockerCfg types.DockerProviderConfig) ([]container.Summary, error) {
	dockerClient, err := NewClient(dockerCfg)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	return dockerClient.ContainerList(ctx, listOptions)
}

func IsErrConnectionFailed(err error) bool {
	return client.IsErrConnectionFailed(err)
}
