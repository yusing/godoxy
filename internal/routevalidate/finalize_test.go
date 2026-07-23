package routevalidate

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	"github.com/yusing/godoxy/internal/route"
)

func TestPreferredPort(t *testing.T) {
	ports := docker.PortMapping{
		22:   {PrivatePort: 22},
		1000: {PrivatePort: 1000},
		3000: {PrivatePort: 80},
	}

	port := preferredPort(ports)
	require.Equal(t, 3000, port)
}

func TestDockerRouteWithResolvablePortIsNotExcludedBeforeFinalize(t *testing.T) {
	r := &route.Route{
		Alias: "app",
		Metadata: route.Metadata{
			Container: &docker.Container{
				Image:           &docker.Image{Name: "custom-app"},
				PrivateHostname: "172.18.0.2",
				PrivatePortMapping: docker.PortMapping{
					8080: container.Port{PrivatePort: 8080, Type: "tcp"},
				},
			},
		},
	}

	require.False(t, r.ShouldExclude())

	finalize(t.Context(), r)

	require.False(t, r.ShouldExclude())
	require.Equal(t, 8080, r.Port.Proxy)
}
