package runtime

import (
	"encoding/json/v2"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestNotifyConfig(t *testing.T) {
	defaults := &IdlewatcherNotifyConfig{To: []string{"gotify"}}
	for _, tc := range []struct {
		name, yaml string
		defaults   *IdlewatcherNotifyConfig
		want       bool
		to         []string
		wire       string
	}{
		{name: "off by default", wire: `{}`},
		{name: "named providers", yaml: "notify: {to: [ntfy]}", want: true, to: []string{"ntfy"}, wire: `{"notify":{"to":["ntfy"]}}`},
		{name: "broadcast", yaml: "notify: {}", want: true, wire: `{"notify":{}}`},
		{name: "explicit disable", yaml: "notify: {to: []}", to: []string{}, wire: `{"notify":{"to":[]}}`},
		{name: "inherit", defaults: defaults, want: true, to: []string{"gotify"}, wire: `{"notify":{"to":["gotify"]}}`},
		{name: "empty notify inherits", yaml: "notify: {}", defaults: defaults, want: true, to: []string{"gotify"}, wire: `{"notify":{"to":["gotify"]}}`},
		{name: "route targets override", yaml: "notify: {to: [ntfy]}", defaults: defaults, want: true, to: []string{"ntfy"}, wire: `{"notify":{"to":["ntfy"]}}`},
		{name: "route opts out", yaml: "notify: {to: []}", defaults: defaults, to: []string{}, wire: `{"notify":{"to":[]}}`},
		{name: "global broadcast", defaults: &IdlewatcherNotifyConfig{}, want: true, wire: `{"notify":{}}`},
		{name: "global disable", defaults: &IdlewatcherNotifyConfig{To: []string{}}, to: []string{}, wire: `{"notify":{"to":[]}}`},
		{name: "omitted targets inherit global disable", yaml: "notify: {}", defaults: &IdlewatcherNotifyConfig{To: []string{}}, to: []string{}, wire: `{"notify":{"to":[]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg IdlewatcherConfig
			require.NoError(t, serialization.UnmarshalValidate([]byte("idle_timeout: 30m\n"+tc.yaml), &cfg, yaml.Unmarshal))
			cfg.Notify = cfg.Notify.ApplyDefaults(tc.defaults)
			require.Equal(t, tc.want, cfg.Notify.Wants())
			if cfg.Notify != nil {
				require.Equal(t, tc.to, cfg.Notify.To)
			}

			data, err := json.Marshal(&IdlewatcherDefaults{Notify: cfg.Notify})
			require.NoError(t, err)
			require.JSONEq(t, tc.wire, string(data))

			first := cfg.Notify
			cfg.Notify = cfg.Notify.ApplyDefaults(tc.defaults)
			require.Equal(t, first, cfg.Notify)
			if tc.defaults != nil && len(tc.defaults.To) > 0 && len(cfg.Notify.To) > 0 {
				cfg.Notify.To[0] = "mutated"
				require.Equal(t, "gotify", tc.defaults.To[0])
			}
		})
	}
}
