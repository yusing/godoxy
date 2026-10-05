package runtime

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func ptr[T any](v T) *T { return &v }

func TestNotifyConfig(t *testing.T) {
	defaults := IdlewatcherNotifyConfig{Enabled: ptr(true), To: []string{"gotify"}}
	for _, tc := range []struct {
		name, yaml string
		defaults   IdlewatcherNotifyConfig
		want       bool
		to         []string
	}{
		{name: "off by default"},
		{name: "named providers", yaml: "notify: {to: [ntfy]}", want: true, to: []string{"ntfy"}},
		{name: "broadcast", yaml: "notify: {enabled: true}", want: true},
		{name: "explicit disable", yaml: "notify: {enabled: false, to: [ntfy]}", to: []string{"ntfy"}},
		{name: "inherit", defaults: defaults, want: true, to: []string{"gotify"}},
		{name: "empty notify inherits", yaml: "notify: {}", defaults: defaults, want: true, to: []string{"gotify"}},
		{name: "route targets override", yaml: "notify: {to: [ntfy]}", defaults: defaults, want: true, to: []string{"ntfy"}},
		{name: "route opts out", yaml: "notify: {enabled: false}", defaults: defaults, to: []string{"gotify"}},
		{name: "route opts in", yaml: "notify: {enabled: true}", defaults: IdlewatcherNotifyConfig{Enabled: ptr(false)}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg IdlewatcherConfig
			require.NoError(t, serialization.UnmarshalValidate([]byte("idle_timeout: 30m\n"+tc.yaml), &cfg, yaml.Unmarshal))
			cfg.Notify.ApplyDefaults(tc.defaults)
			require.Equal(t, tc.want, cfg.Notify.Wants())
			require.Equal(t, tc.to, cfg.Notify.To)

			first := cfg.Notify
			cfg.Notify.ApplyDefaults(tc.defaults)
			require.Equal(t, first, cfg.Notify)
			if len(tc.defaults.To) > 0 {
				cfg.Notify.To[0] = "mutated"
				require.Equal(t, "gotify", tc.defaults.To[0])
			}
		})
	}
}
