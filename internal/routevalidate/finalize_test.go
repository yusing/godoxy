package routevalidate

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/docker"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
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

// finalize must not materialize an idlewatcher config on a route that has none,
// which would defeat `json:"idlewatcher,omitempty"` for every non-idle route.
func TestFinalizeLeavesNilIdlewatcherNil(t *testing.T) {
	r := &route.Route{Alias: "app", Host: "10.0.0.5", Port: route.Port{Proxy: 8080}}

	finalize(t.Context(), r)

	require.Nil(t, r.Idlewatcher)
}

// With no config state in the context the defaults are zero, but the route's own
// notify config must still end up resolved.
func TestFinalizeResolvesIdlewatcherNotify(t *testing.T) {
	r := &route.Route{
		Alias: "app",
		Host:  "10.0.0.5",
		Port:  route.Port{Proxy: 8080},
		Idlewatcher: &idlewatcher.IdlewatcherConfig{
			IdlewatcherConfigBase: idlewatcher.IdlewatcherConfigBase{
				IdleTimeout: 30 * time.Minute,
				Notify:      idlewatcher.IdlewatcherNotifyConfig{To: []string{"gotify"}},
			},
		},
	}

	finalize(t.Context(), r)

	require.True(t, r.Idlewatcher.Notify.Wants(idlewatcher.NotifyEventSleep))
	require.True(t, r.Idlewatcher.Notify.Wants(idlewatcher.NotifyEventWake))
	require.False(t, r.Idlewatcher.Notify.Wants(idlewatcher.NotifyEventReady))
}

// A route that configures nothing stays silent even after the defaults merge.
func TestFinalizeLeavesUnconfiguredIdlewatcherNotifyDisabled(t *testing.T) {
	r := &route.Route{
		Alias: "app",
		Host:  "10.0.0.5",
		Port:  route.Port{Proxy: 8080},
		Idlewatcher: &idlewatcher.IdlewatcherConfig{
			IdlewatcherConfigBase: idlewatcher.IdlewatcherConfigBase{IdleTimeout: 30 * time.Minute},
		},
	}

	finalize(t.Context(), r)

	require.False(t, r.Idlewatcher.Notify.Wants(idlewatcher.NotifyEventSleep))
}
