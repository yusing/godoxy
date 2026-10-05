package routevalidate

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/docker"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/goutils/task"
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

type notifyDefaultsState struct {
	config.State
	cfg config.Config
}

func (s notifyDefaultsState) Value() *config.Config { return &s.cfg }

func TestFinalizeIdlewatcherNotify(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		notify                  *idlewatcher.IdlewatcherNotifyConfig
		global, noWatcher, want bool
		to                      []string
	}{
		{name: "non-idle route", global: true, noWatcher: true},
		{name: "off by default"},
		{name: "route opt in", notify: &idlewatcher.IdlewatcherNotifyConfig{To: []string{"ntfy"}}, want: true, to: []string{"ntfy"}},
		{name: "inherit globals", global: true, want: true, to: []string{"gotify"}},
		{name: "route opts out", global: true, notify: &idlewatcher.IdlewatcherNotifyConfig{To: []string{}}, to: []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &route.Route{Alias: "app", Host: "10.0.0.5", Port: route.Port{Proxy: 8080}}
			if !tc.noWatcher {
				r.Idlewatcher = new(idlewatcher.IdlewatcherConfig)
				r.Idlewatcher.Notify = tc.notify
			}
			ctx := t.Context()
			if tc.global {
				parent := task.GetTestTask(t)
				state := notifyDefaultsState{}
				state.cfg.Defaults.Idlewatcher.Notify = &idlewatcher.IdlewatcherNotifyConfig{To: []string{"gotify"}}
				config.SetCtx(parent, state)
				ctx = parent.Context()
			}
			finalize(ctx, r)
			if tc.noWatcher {
				require.Nil(t, r.Idlewatcher)
			} else {
				require.Equal(t, tc.want, r.Idlewatcher.Notify.Wants())
				if r.Idlewatcher.Notify != nil {
					require.Equal(t, tc.to, r.Idlewatcher.Notify.To)
				}
			}
		})
	}
}
