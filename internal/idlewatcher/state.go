package idlewatcher

import (
	"context"
	"time"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
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
	now := time.Now()
	w.storeState(&containerState{
		status:    idlewatcher.ContainerStatusRunning,
		ready:     false,
		startedAt: now,
	})
	w.healthTicker.Reset(idleWakerCheckInterval)
	w.l.Debug().Time("started_at", now).Msg("container starting")
}

func (w *Watcher) setNapping(status idlewatcher.ContainerStatus) {
	w.clearEventHistory() // Clear events on stop/pause
	w.storeState(&containerState{
		status:      status,
		ready:       false,
		startedAt:   time.Time{},
		healthTries: 0,
	})
}

func (w *Watcher) setError(err error) {
	w.storeState(&containerState{
		status:      idlewatcher.ContainerStatusError,
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
