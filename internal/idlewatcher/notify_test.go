package idlewatcher

import (
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/notif"
)

type notifyCfg = idlewatchertypes.IdlewatcherNotifyConfig

func ptr[T any](v T) *T { return &v }

var errBoom = errors.New("boom")

// newNotifyWatcher returns a watcher with cfg resolved and a sink capturing
// every dispatched message.
//
// NewWatcher seeds the phase from the container status it observes, and a
// watcher that starts out asleep correctly swallows a redundant sleep. Being
// awake is the precondition for observing a sleep at all, so start there; the
// seeding itself is covered by the seeded* cases below.
func newNotifyWatcher(t *testing.T, cfg notifyCfg) (*Watcher, *[]*notif.LogMessage) {
	t.Helper()

	w := newTestWatcher(t)
	cfg.ApplyDefaults(notifyCfg{})
	w.cfg.Notify = cfg

	sent := new([]*notif.LogMessage)
	w.notify = func(msg *notif.LogMessage) { *sent = append(*sent, msg) }
	w.notifyPhase.Store(uint32(notifyPhaseAwake))
	return w, sent
}

func titles(msgs []*notif.LogMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, msg.Title)
	}
	return out
}

func requireTitles(t *testing.T, sent []*notif.LogMessage, want []string) {
	t.Helper()
	got := titles(sent)
	require.Len(t, got, len(want), "got %v", got)
	for i, substr := range want {
		require.Contains(t, got[i], substr)
	}
}

func TestNotifyDispatch(t *testing.T) {
	var (
		sleep = func(w *Watcher) { w.setNapping(idlewatchertypes.ContainerStatusStopped) }
		pause = func(w *Watcher) { w.setNapping(idlewatchertypes.ContainerStatusPaused) }
		wake  = func(w *Watcher) { w.setStarting() }
		ready = func(w *Watcher) { w.setReady() }
		fail  = func(w *Watcher) { w.setError(errBoom) }
	)
	to := func(names ...string) notifyCfg { return notifyCfg{To: names} }
	withEvents := func(cfg notifyCfg, events ...idlewatchertypes.IdlewatcherNotifyEvent) notifyCfg {
		cfg.Events = events
		return cfg
	}

	tests := []struct {
		name  string
		cfg   notifyCfg
		phase *notifyPhase
		setup func(*Watcher)
		steps []func(*Watcher)
		want  []string
	}{
		{
			name:  "silent unless opted in",
			cfg:   notifyCfg{},
			steps: []func(*Watcher){wake, ready, sleep, fail},
		},
		{
			name:  "sleep and wake",
			cfg:   to("gotify"),
			steps: []func(*Watcher){sleep, wake},
			want:  []string{"went to sleep", "is waking up"},
		},
		{
			// The second setStarting arrives from the container event stream and is
			// the case lastIdleAction would fail to dedupe, because sendEvent
			// overwrites it with the wake sub-events in between.
			name:  "repeats of the same phase are suppressed",
			cfg:   to("gotify"),
			steps: []func(*Watcher){sleep, sleep, wake, wake, sleep},
			want:  []string{"went to sleep", "is waking up", "went to sleep"},
		},
		{
			name:  "pause has its own wording",
			cfg:   to("gotify"),
			steps: []func(*Watcher){pause},
			want:  []string{"was paused"},
		},
		{
			name:  "a filtered event still advances the phase",
			cfg:   withEvents(to("gotify"), idlewatchertypes.NotifyEventSleep),
			steps: []func(*Watcher){wake, sleep},
			want:  []string{"went to sleep"},
		},
		{
			name:  "ready and error are not in the default set",
			cfg:   to("gotify"),
			phase: ptr(notifyPhaseWaking), // ready follows a wake
			steps: []func(*Watcher){ready, fail},
		},
		{
			name:  "ready and error when selected",
			cfg:   withEvents(to("gotify"), idlewatchertypes.NotifyEventAll),
			phase: ptr(notifyPhaseWaking),
			steps: []func(*Watcher){ready, fail},
			want:  []string{"is awake", "failed to wake"},
		},
		{
			// Dependency watchers start and stop as a side effect of their parent,
			// so reporting them would duplicate every notification.
			name:  "dependency watchers are suppressed",
			cfg:   to("gotify"),
			setup: func(w *Watcher) { w.cfg.IdleTimeout = neverTick },
			steps: []func(*Watcher){sleep, wake},
		},
		{
			// GoDoxy starting next to a running container must not report a wake
			// that happened before it was watching.
			name:  "seeded from a running container",
			cfg:   to("gotify"),
			phase: ptr(initialNotifyPhase(idlewatchertypes.ContainerStatusRunning)),
			steps: []func(*Watcher){wake, sleep},
			want:  []string{"went to sleep"},
		},
		{
			name:  "seeded from a stopped container",
			cfg:   to("gotify"),
			phase: ptr(initialNotifyPhase(idlewatchertypes.ContainerStatusStopped)),
			steps: []func(*Watcher){sleep, wake},
			want:  []string{"is waking up"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, sent := newNotifyWatcher(t, tc.cfg)
			if tc.setup != nil {
				tc.setup(w)
			}
			if tc.phase != nil {
				w.notifyPhase.Store(uint32(*tc.phase))
			}
			for _, step := range tc.steps {
				step(w)
			}
			requireTitles(t, *sent, tc.want)
		})
	}
}

func TestInitialNotifyPhase(t *testing.T) {
	require.Equal(t, notifyPhaseWaking, initialNotifyPhase(idlewatchertypes.ContainerStatusRunning))
	require.Equal(t, notifyPhaseAsleep, initialNotifyPhase(idlewatchertypes.ContainerStatusStopped))
	require.Equal(t, notifyPhaseAsleep, initialNotifyPhase(idlewatchertypes.ContainerStatusPaused))
}

func TestNotifyExplicitDisableBeatsDefaults(t *testing.T) {
	w := newTestWatcher(t)
	cfg := notifyCfg{Enabled: ptr(false)}
	cfg.ApplyDefaults(notifyCfg{Enabled: ptr(true), To: []string{"gotify"}})
	w.cfg.Notify = cfg

	sent := 0
	w.notify = func(*notif.LogMessage) { sent++ }
	w.notifyPhase.Store(uint32(notifyPhaseAwake))

	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	w.setStarting()

	require.Zero(t, sent)
}

func TestNotifyProviderTargeting(t *testing.T) {
	t.Run("named providers", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, notifyCfg{To: []string{"gotify", "ntfy"}})
		w.setNapping(idlewatchertypes.ContainerStatusStopped)

		require.Len(t, *sent, 1)
		require.Equal(t, []string{"gotify", "ntfy"}, (*sent)[0].To)
	})

	t.Run("none named broadcasts", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, notifyCfg{Enabled: ptr(true)})
		w.setNapping(idlewatchertypes.ContainerStatusStopped)

		require.Len(t, *sent, 1)
		require.Empty(t, (*sent)[0].To)
	})
}

// sleep_failed is a repeated failure, not a state transition, so it must not be
// edge triggered.
func TestNotifySleepFailedBypassesPhase(t *testing.T) {
	w, sent := newNotifyWatcher(t, notifyCfg{
		To:     []string{"gotify"},
		Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventSleepFailed},
	})

	w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", errBoom)
	w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", errBoom)

	requireTitles(t, *sent, []string{"failed to sleep", "failed to sleep"})
	require.Equal(t, zerolog.WarnLevel, (*sent)[0].Level)
	require.Equal(t, notif.ColorError, (*sent)[0].Color)
}

func TestNotifyMessageBody(t *testing.T) {
	fieldsOf := func(msg *notif.LogMessage) map[string]string {
		body, ok := msg.Body.(notif.FieldsBody)
		require.True(t, ok, "body should be a FieldsBody")
		out := make(map[string]string, len(body))
		for _, field := range body {
			out[field.Name] = field.Value
		}
		return out
	}

	t.Run("sleep carries status and idle timeout", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, notifyCfg{To: []string{"gotify"}})
		w.cfg.IdleTimeout = 30 * time.Minute

		w.setNapping(idlewatchertypes.ContainerStatusStopped)

		require.Len(t, *sent, 1)
		got := fieldsOf((*sent)[0])
		require.Equal(t, w.cfg.ContainerName(), got["Container"])
		require.Equal(t, string(idlewatchertypes.ContainerStatusStopped), got["Status"])
		require.Equal(t, "30 minutes", got["Idle Timeout"])
		require.NotEmpty(t, got["Route"])
		require.NotEmpty(t, got["Time"])
		require.Equal(t, zerolog.InfoLevel, (*sent)[0].Level)
	})

	t.Run("error carries the cause", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, notifyCfg{
			To:     []string{"gotify"},
			Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventError},
		})
		w.notifyPhase.Store(uint32(notifyPhaseWaking))

		w.setError(errors.New("container exited with code 1"))

		require.Len(t, *sent, 1)
		require.Equal(t, "container exited with code 1", fieldsOf((*sent)[0])["Error"])
	})

	// newTestWatcher leaves route nil, as dependency watchers can.
	t.Run("nil route falls back to the container name", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, notifyCfg{To: []string{"gotify"}})
		require.Nil(t, w.route)

		w.setNapping(idlewatchertypes.ContainerStatusStopped)

		requireTitles(t, *sent, []string{w.cfg.ContainerName()})
	})
}

func TestNotifyWithoutNotifierIsSafe(t *testing.T) {
	w := newTestWatcher(t)
	cfg := notifyCfg{To: []string{"gotify"}}
	cfg.ApplyDefaults(notifyCfg{})
	w.cfg.Notify = cfg
	require.Nil(t, w.notify)

	require.NotPanics(t, func() {
		w.setNapping(idlewatchertypes.ContainerStatusStopped)
		w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", errBoom)
	})
}
