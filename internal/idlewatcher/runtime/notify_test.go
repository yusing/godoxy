package runtime

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func ptr[T any](v T) *T { return &v }

func TestNotifyResolveEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  IdlewatcherNotifyConfig
		want bool
	}{
		{
			name: "zero value is disabled",
			cfg:  IdlewatcherNotifyConfig{},
			want: false,
		},
		{
			name: "naming providers opts in",
			cfg:  IdlewatcherNotifyConfig{To: []string{"gotify"}},
			want: true,
		},
		{
			name: "explicit enable without providers broadcasts",
			cfg:  IdlewatcherNotifyConfig{Enabled: ptr(true)},
			want: true,
		},
		{
			name: "explicit disable beats a populated to",
			cfg:  IdlewatcherNotifyConfig{Enabled: ptr(false), To: []string{"gotify"}},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg
			cfg.resolve()
			require.Equal(t, tc.want, cfg.Wants(NotifyEventSleep))
		})
	}
}

func TestNotifyWantsEventMask(t *testing.T) {
	all := []IdlewatcherNotifyEvent{
		NotifyEventSleep, NotifyEventWake, NotifyEventReady,
		NotifyEventError, NotifyEventSleepFailed,
	}

	t.Run("default set is exactly sleep and wake", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{To: []string{"gotify"}}
		cfg.resolve()

		require.Equal(t, NotifyEventsDefault, cfg.Events)
		for _, event := range all {
			want := event == NotifyEventSleep || event == NotifyEventWake
			require.Equalf(t, want, cfg.Wants(event), "event %q", event)
		}
	})

	t.Run("all selects every event", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{To: []string{"gotify"}, Events: []IdlewatcherNotifyEvent{NotifyEventAll}}
		cfg.resolve()

		for _, event := range all {
			require.Truef(t, cfg.Wants(event), "event %q", event)
		}
	})

	t.Run("narrow set excludes the rest", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{To: []string{"gotify"}, Events: []IdlewatcherNotifyEvent{NotifyEventError}}
		cfg.resolve()

		require.True(t, cfg.Wants(NotifyEventError))
		require.False(t, cfg.Wants(NotifyEventSleep))
		require.False(t, cfg.Wants(NotifyEventWake))
	})

	t.Run("disabled never wants anything", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{Enabled: ptr(false), Events: []IdlewatcherNotifyEvent{NotifyEventAll}}
		cfg.resolve()

		for _, event := range all {
			require.Falsef(t, cfg.Wants(event), "event %q", event)
		}
	})

	t.Run("nil receiver is safe", func(t *testing.T) {
		var cfg *IdlewatcherNotifyConfig
		require.False(t, cfg.Wants(NotifyEventSleep))
	})
}

func TestNotifyApplyDefaults(t *testing.T) {
	tests := []struct {
		name        string
		route       IdlewatcherNotifyConfig
		defaults    IdlewatcherNotifyConfig
		wantEnabled bool
		wantTo      []string
		wantEvents  []IdlewatcherNotifyEvent
	}{
		{
			name:        "empty route inherits everything",
			route:       IdlewatcherNotifyConfig{},
			defaults:    IdlewatcherNotifyConfig{To: []string{"gotify"}, Events: []IdlewatcherNotifyEvent{NotifyEventReady}},
			wantEnabled: true,
			wantTo:      []string{"gotify"},
			wantEvents:  []IdlewatcherNotifyEvent{NotifyEventReady},
		},
		{
			name:        "route to overrides defaults to",
			route:       IdlewatcherNotifyConfig{To: []string{"ntfy"}},
			defaults:    IdlewatcherNotifyConfig{To: []string{"gotify"}},
			wantEnabled: true,
			wantTo:      []string{"ntfy"},
			wantEvents:  NotifyEventsDefault,
		},
		{
			name:        "route opts out of an enabled global default",
			route:       IdlewatcherNotifyConfig{Enabled: ptr(false)},
			defaults:    IdlewatcherNotifyConfig{Enabled: ptr(true), To: []string{"gotify"}},
			wantEnabled: false,
			wantTo:      []string{"gotify"},
			wantEvents:  NotifyEventsDefault,
		},
		{
			name:        "route opts in to a disabled global default",
			route:       IdlewatcherNotifyConfig{Enabled: ptr(true)},
			defaults:    IdlewatcherNotifyConfig{},
			wantEnabled: true,
			wantTo:      nil,
			wantEvents:  NotifyEventsDefault,
		},
		{
			name:        "no config anywhere stays disabled",
			route:       IdlewatcherNotifyConfig{},
			defaults:    IdlewatcherNotifyConfig{},
			wantEnabled: false,
			wantTo:      nil,
			wantEvents:  NotifyEventsDefault,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.route
			cfg.ApplyDefaults(tc.defaults)

			require.Equal(t, tc.wantEnabled, cfg.enabled)
			require.Equal(t, tc.wantTo, cfg.To)
			require.Equal(t, tc.wantEvents, cfg.Events)
		})
	}
}

func TestNotifyApplyDefaultsIsIdempotent(t *testing.T) {
	defaults := IdlewatcherNotifyConfig{To: []string{"gotify"}, Events: []IdlewatcherNotifyEvent{NotifyEventSleep}}

	cfg := IdlewatcherNotifyConfig{}
	cfg.ApplyDefaults(defaults)
	first := cfg

	cfg.ApplyDefaults(defaults)

	require.Equal(t, first, cfg)
	require.True(t, cfg.Wants(NotifyEventSleep))
	require.False(t, cfg.Wants(NotifyEventWake))
}

func TestNotifyApplyDefaultsDoesNotAliasDefaults(t *testing.T) {
	defaults := IdlewatcherNotifyConfig{To: []string{"gotify"}, Events: []IdlewatcherNotifyEvent{NotifyEventSleep}}

	cfg := IdlewatcherNotifyConfig{}
	cfg.ApplyDefaults(defaults)
	cfg.To[0] = "mutated"
	cfg.Events[0] = NotifyEventReady

	require.Equal(t, []string{"gotify"}, defaults.To)
	require.Equal(t, []IdlewatcherNotifyEvent{NotifyEventSleep}, defaults.Events)
}

func TestNotifyValidate(t *testing.T) {
	t.Run("normalizes case and whitespace", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{Events: []IdlewatcherNotifyEvent{" SLEEP ", "Wake"}}
		require.NoError(t, cfg.Validate())
		require.Equal(t, []IdlewatcherNotifyEvent{NotifyEventSleep, NotifyEventWake}, cfg.Events)
	})

	t.Run("rejects an unknown event", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{Events: []IdlewatcherNotifyEvent{"slep"}}
		err := cfg.Validate()
		require.ErrorIs(t, err, ErrInvalidNotifyEvent)
		require.Contains(t, err.Error(), "slep")
	})

	t.Run("accepts an empty event list", func(t *testing.T) {
		cfg := IdlewatcherNotifyConfig{To: []string{"gotify"}}
		require.NoError(t, cfg.Validate())
		require.Equal(t, NotifyEventsDefault, cfg.Events)
	})
}

// Notify must resolve even for a route with idle_timeout: 0, which short
// circuits IdlewatcherConfig.validate before any per-field work.
func TestNotifyResolvedWithZeroIdleTimeout(t *testing.T) {
	cfg := new(IdlewatcherConfig)
	cfg.Notify = IdlewatcherNotifyConfig{To: []string{"gotify"}}

	require.NoError(t, cfg.Validate())
	require.Equal(t, NotifyEventsDefault, cfg.Notify.Events)
	require.True(t, cfg.Notify.Wants(NotifyEventSleep))
}

// Exercises the real deserialization path used by both YAML routes and Docker
// labels, including the CustomValidator hook on the nested struct.
func TestNotifyDeserialization(t *testing.T) {
	t.Run("nested map from yaml", func(t *testing.T) {
		cfg := new(IdlewatcherConfig)
		err := serialization.MapUnmarshalValidate(map[string]any{
			"idle_timeout": "30m",
			"notify": map[string]any{
				"enabled": true,
				"to":      []any{"gotify", "ntfy"},
				"events":  []any{"sleep", "ready"},
			},
		}, cfg)
		require.NoError(t, err)

		require.Equal(t, []string{"gotify", "ntfy"}, cfg.Notify.To)
		require.Equal(t, []IdlewatcherNotifyEvent{NotifyEventSleep, NotifyEventReady}, cfg.Notify.Events)
		require.True(t, cfg.Notify.Wants(NotifyEventReady))
		require.False(t, cfg.Notify.Wants(NotifyEventWake))
	})

	t.Run("comma separated strings from docker labels", func(t *testing.T) {
		cfg := new(IdlewatcherConfig)
		err := serialization.MapUnmarshalValidate(map[string]any{
			"idle_timeout": "30m",
			"notify": map[string]any{
				"enabled": "false",
				"to":      "gotify, ntfy",
				"events":  "sleep,wake",
			},
		}, cfg)
		require.NoError(t, err)

		require.NotNil(t, cfg.Notify.Enabled)
		require.False(t, *cfg.Notify.Enabled)
		require.Equal(t, []string{"gotify", "ntfy"}, cfg.Notify.To)
		require.Equal(t, []IdlewatcherNotifyEvent{NotifyEventSleep, NotifyEventWake}, cfg.Notify.Events)
	})

	t.Run("unknown event is rejected", func(t *testing.T) {
		cfg := new(IdlewatcherConfig)
		err := serialization.MapUnmarshalValidate(map[string]any{
			"idle_timeout": "30m",
			"notify":       map[string]any{"events": "slep"},
		}, cfg)
		require.ErrorIs(t, err, ErrInvalidNotifyEvent)
	})
}

// Guards the `defaults.idlewatcher` shape documented in config.example.yml.
func TestIdlewatcherDefaultsDeserialization(t *testing.T) {
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(`
notify:
  enabled: true
  to: [gotify, ntfy]
  events: [sleep, wake]
`), &raw))

	var defaults IdlewatcherDefaults
	require.NoError(t, serialization.MapUnmarshalValidate(raw, &defaults))

	require.NotNil(t, defaults.Notify.Enabled)
	require.True(t, *defaults.Notify.Enabled)
	require.Equal(t, []string{"gotify", "ntfy"}, defaults.Notify.To)
	require.Equal(t, NotifyEventsDefault, defaults.Notify.Events)

	// A route with nothing configured inherits the whole thing.
	route := IdlewatcherNotifyConfig{}
	route.ApplyDefaults(defaults.Notify)
	require.True(t, route.Wants(NotifyEventSleep))
	require.True(t, route.Wants(NotifyEventWake))
	require.False(t, route.Wants(NotifyEventReady))

	// A route can opt back out of an enabled global default.
	optedOut := IdlewatcherNotifyConfig{Enabled: ptr(false)}
	optedOut.ApplyDefaults(defaults.Notify)
	require.False(t, optedOut.Wants(NotifyEventSleep))
}
