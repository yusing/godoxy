package docker

import (
	"encoding/json/v2"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/types"
	expect "github.com/yusing/goutils/testing"
)

func TestContainerExplicit(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]string
		isExplicit bool
	}{
		{
			name: "explicit",
			labels: map[string]string{
				"proxy.aliases": "foo",
			},
			isExplicit: true,
		},
		{
			name: "explicit2",
			labels: map[string]string{
				"proxy.idle_timeout": "1s",
			},
			isExplicit: true,
		},
		{
			name:       "not explicit",
			labels:     map[string]string{},
			isExplicit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromDocker(t.Context(), &container.Summary{Names: []string{"test"}, State: "test", Labels: tt.labels}, types.DockerProviderConfig{})
			expect.Equal(t, c.IsExplicit, tt.isExplicit)
		})
	}
}

func TestContainerExclude(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantFlags ExcludeFlag
		wantError string
	}{
		{name: "boolean true", value: "true", wantFlags: ExcludeProxy},
		{name: "boolean false", value: "false"},
		{name: "proxy", value: "proxy", wantFlags: ExcludeProxy},
		{name: "healthcheck", value: "healthcheck", wantFlags: ExcludeHealthCheck},
		{name: "proxy and healthcheck", value: " proxy, healthcheck ", wantFlags: ExcludeAll},
		{name: "all", value: "all", wantFlags: ExcludeAll},
		{name: "unknown", value: "unknown", wantError: "expected a boolean or a comma-separated list"},
		{name: "empty", wantError: "expected a boolean or a comma-separated list"},
		{name: "all with proxy", value: "all,proxy", wantError: "all cannot be combined with other values"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[string]string{LabelExclude: tt.value}
			c := FromDocker(t.Context(), &container.Summary{
				Names:  []string{"test"},
				State:  "created",
				Labels: labels,
			}, types.DockerProviderConfig{})

			expect.Equal(t, c.ExcludeFlags, tt.wantFlags)
			_, excludeLabelRetained := labels[LabelExclude]
			expect.False(t, excludeLabelRetained)
			if tt.wantError == "" {
				expect.Nil(t, c.Errors)
			} else {
				expect.ErrorContains(t, c.Errors, tt.wantError)
			}
		})
	}

	t.Run("absent", func(t *testing.T) {
		c := FromDocker(t.Context(), &container.Summary{
			Names:  []string{"test"},
			State:  "created",
			Labels: map[string]string{},
		}, types.DockerProviderConfig{})
		expect.Equal(t, c.ExcludeFlags, ExcludeFlag(0))
		expect.Nil(t, c.Errors)
	})
}

func TestContainerExcludeJSON(t *testing.T) {
	tests := []struct {
		flags ExcludeFlag
		want  string
	}{
		{want: "none"},
		{flags: ExcludeProxy, want: "proxy"},
		{flags: ExcludeHealthCheck, want: "healthcheck"},
		{flags: ExcludeAll, want: "all"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			data := expect.Must(json.Marshal(&Container{ExcludeFlags: tt.flags}))
			var decoded map[string]any
			expect.NoError(t, json.Unmarshal(data, &decoded))
			exclude, ok := decoded["exclude"].(string)
			expect.True(t, ok)
			expect.Equal(t, exclude, tt.want)
			_, hasLegacyField := decoded["is_excluded"]
			expect.False(t, hasLegacyField)
		})
	}
}

func TestContainerHostNetworkMode(t *testing.T) {
	tests := []struct {
		name              string
		container         *container.Summary
		isHostNetworkMode bool
	}{
		{
			name: "host network mode",
			container: &container.Summary{
				Names: []string{"test"},
				State: "test",
				HostConfig: struct {
					NetworkMode string            "json:\",omitempty\""
					Annotations map[string]string "json:\",omitempty\""
				}{
					NetworkMode: "host",
					Annotations: map[string]string{},
				},
			},
			isHostNetworkMode: true,
		},
		{
			name: "not host network mode",
			container: &container.Summary{
				Names: []string{"test"},
				State: "test",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := FromDocker(t.Context(), tt.container, types.DockerProviderConfig{})
			expect.Equal(t, c.IsHostNetworkMode, tt.isHostNetworkMode)
		})
	}
}

func TestContainerHealthCheckEnabled(t *testing.T) {
	tests := []struct {
		status  string
		enabled bool
	}{
		{status: "Up 5 seconds (health: starting)", enabled: true},
		{status: "Up 10 seconds (healthy)", enabled: true},
		{status: "Up 20 seconds (unhealthy)", enabled: true},
		{status: "Up 30 seconds"},
		{status: "Exited (0) 1 minute ago"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			c := FromDocker(t.Context(), &container.Summary{Names: []string{"test"}, Status: tt.status}, types.DockerProviderConfig{})
			expect.Equal(t, c.HealthCheckEnabled, tt.enabled)
		})
	}
}

func TestImageNameParsing(t *testing.T) {
	tests := []struct {
		full   string
		author string
		image  string
		tag    string
	}{
		{
			full:   "ghcr.io/tensorchord/pgvecto-rs",
			author: "ghcr.io/tensorchord",
			image:  "pgvecto-rs",
			tag:    "latest",
		},
		{
			full:   "redis:latest",
			author: "library",
			image:  "redis",
			tag:    "latest",
		},
		{
			full:   "redis:7.4.0-alpine",
			author: "library",
			image:  "redis",
			tag:    "7.4.0-alpine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.full, func(t *testing.T) {
			helper := containerHelper{&container.Summary{Image: tt.full}}
			im := helper.parseImage()
			expect.Equal(t, im.Author, tt.author)
			expect.Equal(t, im.Name, tt.image)
			expect.Equal(t, im.Tag, tt.tag)
		})
	}
}

func idlewatcherFromLabels(t *testing.T, labels map[string]string) *Container {
	t.Helper()
	return FromDocker(t.Context(), &container.Summary{
		Names:  []string{"test"},
		State:  "running",
		Labels: labels,
	}, types.DockerProviderConfig{})
}

func TestIdlewatcherNotifyLabels(t *testing.T) {
	tests := []struct {
		name       string
		labels     map[string]string
		wantConfig bool
		wantTo     []string
		wantEnable *bool
	}{
		{
			name:       "providers",
			labels:     map[string]string{"proxy.idle_timeout": "30m", "proxy.idle_notify_to": "gotify, ntfy"},
			wantConfig: true,
			wantTo:     []string{"gotify", "ntfy"},
		},
		{
			name:       "explicit opt out",
			labels:     map[string]string{"proxy.idle_timeout": "30m", "proxy.idle_notify": "false"},
			wantConfig: true,
			wantEnable: ptrTo(false),
		},
		{
			name:       "explicit opt in without providers",
			labels:     map[string]string{"proxy.idle_timeout": "30m", "proxy.idle_notify": "true"},
			wantConfig: true,
			wantEnable: ptrTo(true),
		},
		{
			// The idlewatcher config is only built when proxy.idle_timeout is
			// present, so notify labels on their own are dropped with the rest.
			name:   "without idle_timeout",
			labels: map[string]string{"proxy.idle_notify_to": "gotify"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := idlewatcherFromLabels(t, tc.labels)

			require.Nil(t, c.Errors)
			if !tc.wantConfig {
				require.Nil(t, c.IdlewatcherConfig)
				return
			}
			require.NotNil(t, c.IdlewatcherConfig)
			require.Equal(t, tc.wantTo, c.IdlewatcherConfig.Notify.To)
			require.Equal(t, tc.wantEnable, c.IdlewatcherConfig.Notify.Enabled)

			// All idlewatcher labels are consumed, so none may leak into per-alias
			// route field parsing.
			for lbl := range tc.labels {
				require.NotContains(t, c.Labels, lbl)
			}
		})
	}
}

func ptrTo[T any](v T) *T { return &v }

func TestSetNestedKey(t *testing.T) {
	m := map[string]any{}

	setNestedKey(m, "idle_timeout", "30m")
	setNestedKey(m, "notify.to", "gotify")
	setNestedKey(m, "a.b.c", "deep")

	require.Equal(t, "30m", m["idle_timeout"])
	require.Equal(t, map[string]any{"to": "gotify"}, m["notify"])
	require.Equal(t, map[string]any{"b": map[string]any{"c": "deep"}}, m["a"])
}
