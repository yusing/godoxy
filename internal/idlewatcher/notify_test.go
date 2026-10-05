package idlewatcher

import (
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/notif"
)

func TestNotifyTransitions(t *testing.T) {
	for _, status := range []idlewatcher.ContainerStatus{
		idlewatcher.ContainerStatusRunning,
		idlewatcher.ContainerStatusStopped,
		idlewatcher.ContainerStatusPaused,
	} {
		t.Run(string(status), func(t *testing.T) {
			w := newTestWatcher(t)
			w.cfg.Notify.To = []string{"gotify", "ntfy"}
			w.cfg.IdleTimeout = 30 * time.Minute
			var sent []*notif.LogMessage
			w.notify = func(msg *notif.LogMessage) { sent = append(sent, msg) }

			// Initial status stores stay silent, and redundant provider events
			// must not report transitions that happened before startup.
			w.storeState(&containerState{status: status})
			if status == idlewatcher.ContainerStatusRunning {
				w.setStarting()
			} else {
				w.setNapping(status)
			}
			require.Empty(t, sent)

			w.setStarting()
			w.sendEvent(WakeEventWaitingReady, "waiting", nil)
			w.setStarting()
			w.setReady()
			w.setError(errors.New("health check failed"))
			w.setNapping(idlewatcher.ContainerStatusPaused)
			w.setNapping(idlewatcher.ContainerStatusStopped)
			w.setStarting()
			if status != idlewatcher.ContainerStatusRunning {
				require.Contains(t, sent[0].Title, "is waking up")
				sent = sent[1:]
			}
			require.Len(t, sent, 2)
			require.Contains(t, sent[0].Title, w.cfg.ContainerName()+" was paused")
			require.Contains(t, sent[1].Title, "is waking up")
			require.Equal(t, w.cfg.Notify.To, sent[0].To)
			require.Equal(t, zerolog.InfoLevel, sent[0].Level)
			require.Equal(t, notif.ColorInfo, sent[0].Color)
			fields := map[string]string{}
			for _, field := range sent[0].Body.(notif.FieldsBody) {
				fields[field.Name] = field.Value
			}
			require.Equal(t, w.cfg.ContainerName(), fields["Container"])
			require.Equal(t, w.cfg.ContainerName(), fields["Route"])
			require.Equal(t, "paused", fields["Status"])
			require.Equal(t, "30 minutes", fields["Idle Timeout"])
			require.NotEmpty(t, fields["Time"])

			// Reload and teardown stores are silent as well.
			w.storeState(&containerState{status: idlewatcher.ContainerStatusStopped})
			require.Len(t, sent, 2)
		})
	}
}

func TestNotifyOptIn(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cfg        idlewatcher.IdlewatcherNotifyConfig
		dependency bool
		want       int
	}{
		{name: "disabled by default"},
		{name: "broadcast", cfg: idlewatcher.IdlewatcherNotifyConfig{Enabled: new(true)}, want: 1},
		{name: "opt out", cfg: idlewatcher.IdlewatcherNotifyConfig{Enabled: new(false), To: []string{"gotify"}}},
		{name: "dependency", cfg: idlewatcher.IdlewatcherNotifyConfig{To: []string{"gotify"}}, dependency: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWatcher(t)
			w.cfg.Notify = tc.cfg
			if tc.dependency {
				w.cfg.IdleTimeout = neverTick
			}
			var sent []*notif.LogMessage
			w.notify = func(msg *notif.LogMessage) { sent = append(sent, msg) }
			w.storeState(&containerState{status: idlewatcher.ContainerStatusRunning})
			w.setNapping(idlewatcher.ContainerStatusStopped)
			require.Len(t, sent, tc.want)
			if tc.want > 0 {
				require.Contains(t, sent[0].Title, "went to sleep")
				require.Empty(t, sent[0].To)
			}
		})
	}
}

// A channel sink also checks the constructor's runtime notifier binding.
type notifySink chan *notif.LogMessage

func (s notifySink) Notify(msg *notif.LogMessage) { s <- msg }

func TestNewWatcherNotifyReload(t *testing.T) {
	_, parent, mainRoute, _ := newDependencyReloadTest(t, "notify-fixture", nil)
	sink := make(notifySink, 2)
	notif.SetCtx(parent, sink)
	cfg := idlewatcherTestConfig("notify", nil)
	cfg.Notify.To = []string{"gotify"}
	r := newIdlewatcherTestRoute("notify-route", mainRoute.provider, cfg)
	w, err := NewWatcher(parent, r, cfg)
	require.NoError(t, err)
	require.Empty(t, sink)
	w.setStarting()
	require.Len(t, sink, 1)
	msg := <-sink
	require.Contains(t, msg.Title, "notify-route is waking up")
	require.Equal(t, []string{"gotify"}, msg.To)

	cfg = idlewatcherTestConfig("notify", nil)
	cfg.Notify.To = []string{"ntfy"}
	reloaded, err := NewWatcher(parent, r, cfg)
	require.NoError(t, err)
	require.Same(t, w, reloaded)
	require.Empty(t, sink, "reloading the observed container state is silent")
	w.setStarting()
	require.Len(t, sink, 1)
	require.Equal(t, []string{"ntfy"}, (<-sink).To)
}
