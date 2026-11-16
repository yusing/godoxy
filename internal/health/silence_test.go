package health

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	strutils "github.com/yusing/goutils/strings"
)

func TestSilenceWindowParse(t *testing.T) {
	var w SilenceWindow
	require.NoError(t, w.Parse(" 02:00 - 04:30 "))
	require.Equal(t, 2*time.Hour, w.Start)
	require.Equal(t, 4*time.Hour+30*time.Minute, w.End)
	require.Equal(t, "02:00-04:30", w.String())

	for _, bad := range []string{"", "02:00", "2-4", "25:00-01:00", "02:00-02:00", "02:00-03:00-04:00", "2:00-4:00"} {
		require.ErrorIs(t, new(SilenceWindow).Parse(bad), ErrInvalidSilenceWindow, bad)
	}
}

func TestSilenceWindowContains(t *testing.T) {
	day := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, time.Local) }

	var w SilenceWindow
	require.NoError(t, w.Parse("02:00-04:00"))
	require.False(t, w.Contains(day(1, 59)))
	require.True(t, w.Contains(day(2, 0)))
	require.True(t, w.Contains(day(3, 59)))
	require.False(t, w.Contains(day(4, 0)))

	require.NoError(t, w.Parse("22:00-01:00"))
	require.True(t, w.Contains(day(23, 0)))
	require.True(t, w.Contains(day(0, 30)))
	require.False(t, w.Contains(day(1, 0)))
	require.False(t, w.Contains(day(12, 0)))
}

func TestSilenceJSONRoundTrip(t *testing.T) {
	var w SilenceWindow
	require.NoError(t, w.Parse("23:30-00:15"))
	b, err := strutils.MarshalJSON(HealthCheckConfig{Silence: []SilenceWindow{w}})
	require.NoError(t, err)
	require.Contains(t, string(b), `"silence":["23:30-00:15"]`)
}

func TestApplyDefaultsSilence(t *testing.T) {
	var w SilenceWindow
	require.NoError(t, w.Parse("02:00-04:00"))
	defaults := HealthCheckConfig{Silence: []SilenceWindow{w}}

	inherit := HealthCheckConfig{}
	inherit.ApplyDefaults(defaults)
	require.Equal(t, defaults.Silence, inherit.Silence)

	var own SilenceWindow
	require.NoError(t, own.Parse("05:00-06:00"))
	override := HealthCheckConfig{Silence: []SilenceWindow{own}}
	override.ApplyDefaults(defaults)
	require.Equal(t, []SilenceWindow{own}, override.Silence)

	optOut := HealthCheckConfig{Silence: []SilenceWindow{}}
	optOut.ApplyDefaults(defaults)
	require.Empty(t, optOut.Silence)
	require.False(t, optOut.IsSilenced(time.Date(2026, 11, 2, 3, 0, 0, 0, time.Local)))
}
