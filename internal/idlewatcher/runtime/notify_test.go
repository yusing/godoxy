package runtime

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func ptr[T any](v T) *T { return &v }

func TestNotifyEnabled(t *testing.T) {
	tests := []struct {
		name     string
		route    IdlewatcherNotifyConfig
		defaults IdlewatcherNotifyConfig
		want     bool
		wantTo   []string
	}{
		{
			name:  "zero value is disabled",
			route: IdlewatcherNotifyConfig{},
		},
		{
			name:   "naming providers opts in",
			route:  IdlewatcherNotifyConfig{To: []string{"gotify"}},
			want:   true,
			wantTo: []string{"gotify"},
		},
		{
			name:  "explicit enable without providers broadcasts",
			route: IdlewatcherNotifyConfig{Enabled: ptr(true)},
			want:  true,
		},
		{
			name:   "explicit disable beats a populated to",
			route:  IdlewatcherNotifyConfig{Enabled: ptr(false), To: []string{"gotify"}},
			want:   false,
			wantTo: []string{"gotify"},
		},
		{
			name:     "empty route inherits the globals",
			defaults: IdlewatcherNotifyConfig{To: []string{"gotify"}},
			want:     true,
			wantTo:   []string{"gotify"},
		},
		{
			name:     "route to overrides the global to",
			route:    IdlewatcherNotifyConfig{To: []string{"ntfy"}},
			defaults: IdlewatcherNotifyConfig{To: []string{"gotify"}},
			want:     true,
			wantTo:   []string{"ntfy"},
		},
		{
			name:     "route opts out of an enabled global",
			route:    IdlewatcherNotifyConfig{Enabled: ptr(false)},
			defaults: IdlewatcherNotifyConfig{Enabled: ptr(true), To: []string{"gotify"}},
			want:     false,
			wantTo:   []string{"gotify"},
		},
		{
			name:  "route opts in to a disabled global",
			route: IdlewatcherNotifyConfig{Enabled: ptr(true)},
			want:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.route
			cfg.ApplyDefaults(tc.defaults)

			require.Equal(t, tc.want, cfg.Wants())
			require.Equal(t, tc.wantTo, cfg.To)
		})
	}
}

func TestNotifyApplyDefaultsIsIdempotentAndDoesNotAlias(t *testing.T) {
	defaults := IdlewatcherNotifyConfig{To: []string{"gotify"}}

	cfg := IdlewatcherNotifyConfig{}
	cfg.ApplyDefaults(defaults)
	first := cfg
	cfg.ApplyDefaults(defaults)
	require.Equal(t, first, cfg)

	cfg.To[0] = "mutated"
	require.Equal(t, []string{"gotify"}, defaults.To, "defaults must not be aliased")
}

func TestNotifyNilReceiverIsSafe(t *testing.T) {
	var cfg *IdlewatcherNotifyConfig
	require.False(t, cfg.Wants())
}

// Notify must resolve even for a route with idle_timeout: 0, which short
// circuits IdlewatcherConfig.validate before any per-field work.
func TestNotifyResolvedWithZeroIdleTimeout(t *testing.T) {
	cfg := new(IdlewatcherConfig)
	cfg.Notify = IdlewatcherNotifyConfig{To: []string{"gotify"}}

	require.NoError(t, cfg.Validate())
	require.True(t, cfg.Notify.Wants())
}

// Exercises the real deserialization path used by both YAML routes and Docker
// labels, then the defaults merge that routevalidate.finalize performs.
func TestNotifyDeserialization(t *testing.T) {
	tests := []struct {
		name   string
		route  map[string]any
		want   bool
		wantTo []string
	}{
		{
			name:   "nested map from yaml",
			route:  map[string]any{"idle_timeout": "30m", "notify": map[string]any{"enabled": true, "to": []any{"gotify", "ntfy"}}},
			want:   true,
			wantTo: []string{"gotify", "ntfy"},
		},
		{
			name:   "comma separated strings from docker labels",
			route:  map[string]any{"idle_timeout": "30m", "notify": map[string]any{"to": "gotify, ntfy"}},
			want:   true,
			wantTo: []string{"gotify", "ntfy"},
		},
		{
			name:  "explicit opt out from a docker label",
			route: map[string]any{"idle_timeout": "30m", "notify": map[string]any{"enabled": "false"}},
			want:  false,
		},
		{
			name:  "no notify block",
			route: map[string]any{"idle_timeout": "30m"},
			want:  false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := new(IdlewatcherConfig)
			require.NoError(t, serialization.MapUnmarshalValidate(tc.route, cfg))

			cfg.Notify.ApplyDefaults(IdlewatcherNotifyConfig{})

			require.Equal(t, tc.want, cfg.Notify.Wants())
			require.Equal(t, tc.wantTo, cfg.Notify.To)
		})
	}
}

// A route with an idlewatcher block but no notify block must still inherit the
// globals that routevalidate.finalize offers.
func TestNotifyInheritsGlobals(t *testing.T) {
	globals := IdlewatcherNotifyConfig{To: []string{"gotify"}}
	globals.resolve()

	for _, route := range []map[string]any{
		{"idle_timeout": "30m"},
		{"idle_timeout": "30m", "notify": map[string]any{}},
	} {
		cfg := new(IdlewatcherConfig)
		require.NoError(t, serialization.MapUnmarshalValidate(route, cfg))

		cfg.Notify.ApplyDefaults(globals)

		require.True(t, cfg.Notify.Wants())
		require.Equal(t, []string{"gotify"}, cfg.Notify.To)
	}
}

// Guards the `defaults.idlewatcher` shape documented in config.example.yml.
func TestIdlewatcherDefaultsDeserialization(t *testing.T) {
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte("notify:\n  enabled: true\n  to: [gotify, ntfy]\n"), &raw))

	var defaults IdlewatcherDefaults
	require.NoError(t, serialization.MapUnmarshalValidate(raw, &defaults))

	require.NotNil(t, defaults.Notify.Enabled)
	require.True(t, *defaults.Notify.Enabled)
	require.Equal(t, []string{"gotify", "ntfy"}, defaults.Notify.To)
}
