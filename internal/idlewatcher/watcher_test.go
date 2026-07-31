package idlewatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	idlewatchertypes "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

func TestWatchUntilDestroyStopsWhenProviderWatchEnds(t *testing.T) {
	providerErr := errors.New("provider watch failed")
	tests := []struct {
		name     string
		provider *watchUntilDestroyProvider
		wantErr  error
		wantText string
	}{
		{
			name:     "provider error",
			provider: &watchUntilDestroyProvider{err: providerErr},
			wantErr:  providerErr,
		},
		{
			name:     "closed channels",
			provider: &watchUntilDestroyProvider{},
			wantText: "watcher streams closed",
		},
		{
			name: "recovered stream closure",
			provider: &watchUntilDestroyProvider{
				err:   providerErr,
				event: watcherEvents.Event{Action: watcherEvents.ActionForceReload},
			},
			wantText: "watcher streams closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWatchUntilDestroyTestWatcher(t, tt.provider, zerolog.Nop())

			resultCh := make(chan error, 1)
			go func() {
				resultCh <- w.watchUntilDestroy()
			}()

			select {
			case err := <-resultCh:
				if tt.wantErr != nil {
					require.ErrorIs(t, err, tt.wantErr)
				} else {
					require.ErrorContains(t, err, tt.wantText)
				}
			case <-time.After(time.Second):
				w.task.Finish(errors.New("test timed out"))
				<-resultCh
				t.Fatal("watchUntilDestroy did not return after the provider watcher ended")
			}
		})
	}
}

func TestWatchUntilDestroyLogsProviderErrorAndKeepsWatching(t *testing.T) {
	providerErr := errors.New("provider watch failed")
	provider := &watchUntilDestroyProvider{
		err:   providerErr,
		event: watcherEvents.Event{Action: watcherEvents.ActionContainerDestroy},
	}
	var logOutput bytes.Buffer
	w := newWatchUntilDestroyTestWatcher(t, provider, zerolog.New(&logOutput).Level(zerolog.ErrorLevel))

	require.ErrorIs(t, w.watchUntilDestroy(), errCauseContainerDestroy)

	var logEvent map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logOutput.Bytes()), &logEvent))
	require.Equal(t, zerolog.ErrorLevel.String(), logEvent[zerolog.LevelFieldName])
	require.Equal(t, "watcher error", logEvent[zerolog.MessageFieldName])
	require.Equal(t, providerErr.Error(), logEvent[zerolog.ErrorFieldName])
}

func TestWatchUntilDestroyStopsRunningContainerAfterReadinessTimeout(t *testing.T) {
	w := newTestWatcher(t)
	provider := &readinessTimeoutProvider{
		events:  make(chan watcherEvents.Event, 1),
		stopped: make(chan struct{}, 1),
	}
	w.provider.Store(provider)
	w.hc = &unhealthyHealthChecker{
		targetURL: &url.URL{Scheme: "http", Host: "container.test"},
	}
	w.l = zerolog.Nop()
	w.cfg.IdleTimeout = 20 * time.Millisecond
	w.cfg.WakeTimeout = time.Millisecond
	w.cfg.StopMethod = idlewatchertypes.ContainerStopMethodStop
	w.idleTicker.Reset(w.cfg.IdleTimeout)
	w.healthTicker.Reset(time.Millisecond)
	w.state.Store(&containerState{
		status:    idlewatchertypes.ContainerStatusRunning,
		startedAt: time.Now().Add(-time.Second),
	})

	resultCh := make(chan error, 1)
	go func() {
		resultCh <- w.watchUntilDestroy()
	}()

	select {
	case <-provider.stopped:
	case <-time.After(250 * time.Millisecond):
		w.task.Finish(errors.New("test timed out"))
		<-resultCh
		t.Fatal("readiness timeout prevented the idle timer from stopping the running container")
	}

	require.Eventually(t, func() bool {
		state := w.state.Load()
		return state.status == idlewatchertypes.ContainerStatusStopped && state.err == nil
	}, time.Second, time.Millisecond, "container stop event did not clear the readiness error")
	w.task.Finish(nil)
	require.ErrorIs(t, <-resultCh, context.Canceled)
}

func newWatchUntilDestroyTestWatcher(t *testing.T, provider idlewatchertypes.Provider, logger zerolog.Logger) *Watcher {
	t.Helper()

	watcherTask := task.GetTestTask(t).Subtask("watch_until_destroy", true)
	w := &Watcher{
		l:            logger,
		cfg:          idlewatcherTestConfig("watch-until-destroy", nil),
		idleTicker:   time.NewTicker(time.Hour),
		healthTicker: time.NewTicker(time.Hour),
		task:         watcherTask,
	}
	w.provider.Store(provider)
	t.Cleanup(func() {
		w.idleTicker.Stop()
		w.healthTicker.Stop()
		watcherTask.FinishAndWait(nil)
	})
	return w
}

type watchUntilDestroyProvider struct {
	err   error
	event watcherEvents.Event
}

func (*watchUntilDestroyProvider) ContainerPause(context.Context) error   { return nil }
func (*watchUntilDestroyProvider) ContainerUnpause(context.Context) error { return nil }
func (*watchUntilDestroyProvider) ContainerStart(context.Context) error   { return nil }
func (*watchUntilDestroyProvider) ContainerStop(context.Context, idlewatchertypes.Signal, int) error {
	return nil
}
func (*watchUntilDestroyProvider) ContainerKill(context.Context, idlewatchertypes.Signal) error {
	return nil
}
func (*watchUntilDestroyProvider) ContainerStatus(context.Context) (idlewatchertypes.ContainerStatus, error) {
	return idlewatchertypes.ContainerStatusStopped, nil
}
func (p *watchUntilDestroyProvider) Watch(context.Context) (<-chan watcherEvents.Event, <-chan error) {
	eventCh := make(chan watcherEvents.Event)
	errCh := make(chan error)
	go func() {
		defer close(eventCh)
		defer close(errCh)
		if p.err != nil {
			errCh <- p.err
		}
		if p.event.Action != 0 {
			eventCh <- p.event
		}
	}()
	return eventCh, errCh
}
func (*watchUntilDestroyProvider) Close() {}

type readinessTimeoutProvider struct {
	events  chan watcherEvents.Event
	stopped chan struct{}
}

func (*readinessTimeoutProvider) ContainerPause(context.Context) error   { return nil }
func (*readinessTimeoutProvider) ContainerUnpause(context.Context) error { return nil }
func (*readinessTimeoutProvider) ContainerStart(context.Context) error   { return nil }
func (p *readinessTimeoutProvider) ContainerStop(context.Context, idlewatchertypes.Signal, int) error {
	p.events <- watcherEvents.Event{Action: watcherEvents.ActionContainerStop}
	p.stopped <- struct{}{}
	return nil
}
func (*readinessTimeoutProvider) ContainerKill(context.Context, idlewatchertypes.Signal) error {
	return nil
}
func (*readinessTimeoutProvider) ContainerStatus(context.Context) (idlewatchertypes.ContainerStatus, error) {
	return idlewatchertypes.ContainerStatusRunning, nil
}
func (p *readinessTimeoutProvider) Watch(ctx context.Context) (<-chan watcherEvents.Event, <-chan error) {
	errs := make(chan error)
	go func() {
		<-ctx.Done()
		close(p.events)
		close(errs)
	}()
	return p.events, errs
}
func (*readinessTimeoutProvider) Close() {}
