package idlewatcher

import (
	"context"
	"time"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	gevents "github.com/yusing/goutils/events"
)

func (w *Watcher) running() bool {
	return w.state.Load().status == idlewatcher.ContainerStatusRunning
}

func (w *Watcher) ready() bool {
	return w.state.Load().ready
}

func (w *Watcher) error() error {
	return w.state.Load().err
}

func (w *Watcher) storeState(state *containerState) {
	w.stateChangedMu.Lock()
	w.state.Store(state)
	close(w.stateChangedCh)
	w.stateChangedCh = make(chan struct{})
	w.stateChangedMu.Unlock()
}

func (w *Watcher) setReady() {
	w.storeState(&containerState{
		status: idlewatcher.ContainerStatusRunning,
		ready:  true,
	})
	// Send ready event via SSE
	w.sendEvent(WakeEventReady, w.cfg.ContainerName()+" is ready!", nil)
}

func (w *Watcher) setStarting() {
	alreadyStarting := w.wakeInProgress()
	now := time.Now()
	w.storeState(&containerState{
		status:    idlewatcher.ContainerStatusRunning,
		ready:     false,
		startedAt: now,
	})
	w.healthTicker.Reset(idleWakerCheckInterval)
	w.l.Debug().Time("started_at", now).Msg("container starting")
	if !alreadyStarting {
		w.emitIdleActivity(gevents.LevelInfo, IdleEventActionStarting, w.cfg.ContainerName()+" is starting...", nil)
	}
}

func (w *Watcher) setNapping(status idlewatcher.ContainerStatus) {
	w.clearEventHistory() // Clear events on stop/pause
	w.storeState(&containerState{
		status:      status,
		ready:       false,
		startedAt:   time.Time{},
		healthTries: 0,
	})
	message := w.cfg.ContainerName() + " went to sleep"
	if status == idlewatcher.ContainerStatusPaused {
		message = w.cfg.ContainerName() + " was paused"
	}
	w.emitIdleActivity(gevents.LevelInfo, IdleEventActionNapping, message, nil)
}

func (w *Watcher) setError(err error) {
	state := w.state.Load()
	w.storeState(&containerState{
		status:      state.status,
		ready:       false,
		err:         err,
		startedAt:   time.Time{},
		healthTries: 0,
	})
	w.sendEvent(WakeEventError, "Container error", err)
}

// waitForReady waits for the container to become ready or context to be canceled.
// Returns true if ready, false if canceled.
func (w *Watcher) waitForReady(ctx context.Context) bool {
	for {
		w.stateChangedMu.Lock()
		if w.ready() {
			w.stateChangedMu.Unlock()
			return true
		}
		stateChangedCh := w.stateChangedCh
		w.stateChangedMu.Unlock()

		select {
		case <-stateChangedCh:
			continue
		case <-ctx.Done():
			return false
		}
	}
}

func (w *Watcher) waitStarted(reqCtx context.Context) bool {
	select {
	case <-reqCtx.Done():
		return false
	case <-w.route.Started():
		return true
	}
}
