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
		Container: &docker.Container{
			Image:           &docker.Image{Name: "custom-app"},
			PrivateHostname: "172.18.0.2",
			PrivatePortMapping: docker.PortMapping{
				8080: container.Port{PrivatePort: 8080, Type: "tcp"},
			},
		},
	}

	require.False(t, r.ShouldExclude())

	finalize(t.Context(), r)

	require.False(t, r.ShouldExclude())
	require.Equal(t, 8080, r.Port.Proxy)
}

func TestFinalizeHomepage_ImmichServerUsesImmichCategory(t *testing.T) {
	r := &route.Route{
		Alias: "immich-server",
		Container: &docker.Container{
			ContainerName:   "immich-server",
			Image:           &docker.Image{Name: "immich-server"},
			PrivateHostname: "172.18.0.2",
			PrivatePortMapping: docker.PortMapping{
				2283: container.Port{PrivatePort: 2283, Type: "tcp"},
			},
		},
	}

	finalize(t.Context(), r)

	require.NotNil(t, r.Homepage)
	require.Equal(t, "Media", r.Homepage.Category)
	require.Equal(t, "Immich Server", r.Homepage.Name)
}
