package idlewatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
