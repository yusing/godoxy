package health_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/health"
	"github.com/yusing/godoxy/internal/serialization"
	strutils "github.com/yusing/goutils/strings"
)

func TestSilenceConfigForms(t *testing.T) {
	tests := map[string]struct {
		src  any
		want []string
	}{
		"single string":  {src: "02:00-04:00", want: []string{"02:00-04:00"}},
		"comma label":    {src: "02:00-04:00, 23:30-00:30", want: []string{"02:00-04:00", "23:30-00:30"}},
		"yaml list":      {src: []any{"02:00-04:00", "23:30-00:30"}, want: []string{"02:00-04:00", "23:30-00:30"}},
		"empty opts out": {src: []any{}, want: []string{}},
		"empty label":    {src: "[]", want: []string{}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg health.HealthCheckConfig
			require.NoError(t, serialization.MapUnmarshalValidate(serialization.SerializedObject{"silence": tt.src}, &cfg))
			require.NotNil(t, cfg.Silence)
			got := make([]string, len(cfg.Silence))
			for i, w := range cfg.Silence {
				got[i] = w.String()
			}
			require.Equal(t, tt.want, got)
		})
	}

	var cfg health.HealthCheckConfig
	require.Error(t, serialization.MapUnmarshalValidate(serialization.SerializedObject{"silence": "2am-4am"}, &cfg))
}

func TestSilenceConfigJSONPreservesInheritance(t *testing.T) {
	for _, src := range []string{`{}`, `{"silence":[]}`, `{"silence":["02:00-04:00"]}`} {
		t.Run(src, func(t *testing.T) {
			var cfg health.HealthCheckConfig
			require.NoError(t, strutils.UnmarshalJSON([]byte(src), &cfg))
			data, err := strutils.MarshalJSON(cfg)
			require.NoError(t, err)
			var restored health.HealthCheckConfig
			require.NoError(t, strutils.UnmarshalJSON(data, &restored))
			require.Equal(t, cfg.Silence, restored.Silence)
			if cfg.Silence == nil {
				require.NotContains(t, string(data), `"silence"`)
			}
		})
	}
}
