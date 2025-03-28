package docker

import (
	"context"
	"errors"
	"time"
)

func Inspect(dockerHost string, containerID string) (*Container, error) {
	client, err := NewClient(dockerHost)
	if err != nil {
		return nil, err
	}

	defer client.Close()
	return client.Inspect(containerID)
}

func (c *SharedClient) Inspect(containerID string) (*Container, error) {
	ctx, cancel := context.WithTimeoutCause(context.Background(), 3*time.Second, errors.New("docker container inspect timeout"))
	defer cancel()

	json, err := c.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return FromInspectResponse(json, c.key), nil
}
