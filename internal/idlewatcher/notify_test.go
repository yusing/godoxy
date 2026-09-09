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

func ptr[T any](v T) *T { return &v }

// newNotifyWatcher returns a watcher with the given notify config resolved and
// an injected sink capturing every dispatched message.
func newNotifyWatcher(t *testing.T, cfg idlewatchertypes.IdlewatcherNotifyConfig) (*Watcher, *[]*notif.LogMessage) {
	t.Helper()

	w := newTestWatcher(t)
	cfg.ApplyDefaults(idlewatchertypes.IdlewatcherNotifyConfig{})
	w.cfg.Notify = cfg

	sent := new([]*notif.LogMessage)
	w.notify = func(msg *notif.LogMessage) {
		*sent = append(*sent, msg)
	}
	return w, sent
}

func TestNotifyDisabledByDefault(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{})

	w.setStarting()
	w.setReady()
	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	w.setError(errors.New("boom"))

	require.Empty(t, *sent)
}

func TestNotifySleepAndWakeAreEdgeTriggered(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})

	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	require.Len(t, *sent, 1)
	require.Contains(t, (*sent)[0].Title, "went to sleep")

	// A repeat of the same phase must not notify again.
	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	require.Len(t, *sent, 1)

	w.setStarting()
	require.Len(t, *sent, 2)
	require.Contains(t, (*sent)[1].Title, "is waking up")

	// The second setStarting arriving from the container event stream is the
	// case lastIdleAction would fail to dedupe, because sendEvent overwrites it
	// with the wake sub-events in between.
	w.setStarting()
	require.Len(t, *sent, 2)

	// Going back to sleep is a new edge and must notify.
	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	require.Len(t, *sent, 3)
	require.Contains(t, (*sent)[2].Title, "went to sleep")
}

func TestNotifyPausedUsesItsOwnWording(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})

	w.setNapping(idlewatchertypes.ContainerStatusPaused)

	require.Len(t, *sent, 1)
	require.Contains(t, (*sent)[0].Title, "was paused")
}

// A filtered-out event must still advance the phase, otherwise the next edge
// would be missed.
func TestNotifyEventFilterStillAdvancesPhase(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{
		To:     []string{"gotify"},
		Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventSleep},
	})

	w.setStarting()
	require.Empty(t, *sent, "wake is filtered out")

	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	require.Len(t, *sent, 1, "sleep after a filtered wake is still an edge")
	require.Contains(t, (*sent)[0].Title, "went to sleep")
}

func TestNotifyReadyAndErrorAreOptIn(t *testing.T) {
	t.Run("excluded from the default set", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})

		w.setReady()
		w.setError(errors.New("boom"))

		require.Empty(t, *sent)
	})

	t.Run("delivered when selected", func(t *testing.T) {
		w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{
			To:     []string{"gotify"},
			Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventAll},
		})

		w.setReady()
		w.setError(errors.New("boom"))

		require.Len(t, *sent, 2)
		require.Contains(t, (*sent)[0].Title, "is awake")
		require.Equal(t, zerolog.InfoLevel, (*sent)[0].Level)
		require.Contains(t, (*sent)[1].Title, "failed to wake")
		require.Equal(t, zerolog.WarnLevel, (*sent)[1].Level)
	})
}

func TestNotifyExplicitDisableBeatsDefaults(t *testing.T) {
	w := newTestWatcher(t)
	cfg := idlewatchertypes.IdlewatcherNotifyConfig{Enabled: ptr(false)}
	cfg.ApplyDefaults(idlewatchertypes.IdlewatcherNotifyConfig{Enabled: ptr(true), To: []string{"gotify"}})
	w.cfg.Notify = cfg

	sent := 0
	w.notify = func(*notif.LogMessage) { sent++ }

	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	w.setStarting()

	require.Zero(t, sent)
}

// Dependency watchers are started and stopped as a side effect of their parent,
// so reporting them separately would duplicate every notification.
func TestNotifySuppressedForDependencyWatcher(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})
	w.cfg.IdleTimeout = neverTick

	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	w.setStarting()

	require.Empty(t, *sent)
}

// GoDoxy starting next to an already running container must not report a wake
// that happened before it was watching.
func TestNotifySeededFromRunningContainer(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})
	w.notifyPhase.Store(uint32(initialNotifyPhase(idlewatchertypes.ContainerStatusRunning)))

	w.setStarting()
	require.Empty(t, *sent)

	// A real sleep afterwards is still reported.
	w.setNapping(idlewatchertypes.ContainerStatusStopped)
	require.Len(t, *sent, 1)
}

func TestInitialNotifyPhase(t *testing.T) {
	require.Equal(t, notifyPhaseWaking, initialNotifyPhase(idlewatchertypes.ContainerStatusRunning))
	require.Equal(t, notifyPhaseAsleep, initialNotifyPhase(idlewatchertypes.ContainerStatusStopped))
	require.Equal(t, notifyPhaseAsleep, initialNotifyPhase(idlewatchertypes.ContainerStatusPaused))
}

func TestNotifyTargetsConfiguredProviders(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify", "ntfy"}})

	w.setNapping(idlewatchertypes.ContainerStatusStopped)

	require.Len(t, *sent, 1)
	require.Equal(t, []string{"gotify", "ntfy"}, (*sent)[0].To)
}

func TestNotifyBroadcastsWhenNoProvidersNamed(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{Enabled: ptr(true)})

	w.setNapping(idlewatchertypes.ContainerStatusStopped)

	require.Len(t, *sent, 1)
	require.Empty(t, (*sent)[0].To, "an empty To broadcasts to every provider")
}

// sleep_failed is not a state transition, so repeated failures must each report.
func TestNotifySleepFailedBypassesPhase(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{
		To:     []string{"gotify"},
		Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventSleepFailed},
	})

	err := errors.New("timeout waiting for container to stop")
	w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", err)
	w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", err)

	require.Len(t, *sent, 2)
	require.Contains(t, (*sent)[0].Title, "failed to sleep")
	require.Equal(t, zerolog.WarnLevel, (*sent)[0].Level)
}

func TestNotifyBodyFields(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})
	w.cfg.IdleTimeout = 30 * time.Minute

	w.setNapping(idlewatchertypes.ContainerStatusStopped)

	require.Len(t, *sent, 1)
	body, ok := (*sent)[0].Body.(notif.FieldsBody)
	require.True(t, ok, "body should be a FieldsBody")

	got := make(map[string]string, len(body))
	for _, field := range body {
		got[field.Name] = field.Value
	}
	require.Equal(t, w.cfg.ContainerName(), got["Container"])
	require.Equal(t, string(idlewatchertypes.ContainerStatusStopped), got["Status"])
	require.Equal(t, "30 minutes", got["Idle Timeout"])
	require.NotEmpty(t, got["Route"])
	require.NotEmpty(t, got["Time"])
}

func TestNotifyErrorBodyCarriesTheError(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{
		To:     []string{"gotify"},
		Events: []idlewatchertypes.IdlewatcherNotifyEvent{idlewatchertypes.NotifyEventError},
	})

	w.setError(errors.New("container exited with code 1"))

	require.Len(t, *sent, 1)
	body := (*sent)[0].Body.(notif.FieldsBody)

	var detail string
	for _, field := range body {
		if field.Name == "Error" {
			detail = field.Value
		}
	}
	require.Equal(t, "container exited with code 1", detail)
}

// newTestWatcher leaves route nil, as dependency watchers can.
func TestNotifyNilRouteFallsBackToContainerName(t *testing.T) {
	w, sent := newNotifyWatcher(t, idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}})
	require.Nil(t, w.route)

	w.setNapping(idlewatchertypes.ContainerStatusStopped)

	require.Len(t, *sent, 1)
	require.Contains(t, (*sent)[0].Title, w.cfg.ContainerName())
}

// A watcher with no notifier bound must not panic.
func TestNotifyWithoutNotifierIsSafe(t *testing.T) {
	w := newTestWatcher(t)
	cfg := idlewatchertypes.IdlewatcherNotifyConfig{To: []string{"gotify"}}
	cfg.ApplyDefaults(idlewatchertypes.IdlewatcherNotifyConfig{})
	w.cfg.Notify = cfg
	require.Nil(t, w.notify)

	require.NotPanics(t, func() {
		w.setNapping(idlewatchertypes.ContainerStatusStopped)
		w.notifyOneShot(idlewatchertypes.NotifyEventSleepFailed, "", errors.New("boom"))
	})
}
