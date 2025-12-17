package idlewatcher

import (
	"context"
	"time"

	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/types"
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

func (w *Watcher) setReady() {
	w.state.Store(&containerState{
		status: idlewatcher.ContainerStatusRunning,
		ready:  true,
	})
	// Send ready event via SSE
	w.sendEvent(WakeEventReady, w.cfg.ContainerName()+" is ready!", nil)
	// Notify waiting handlers that container is ready
	select {
	case w.readyNotifyCh <- struct{}{}:
	default: // channel full, notification already pending
	}
}

func (w *Watcher) setStarting() {
	now := time.Now()
	w.state.Store(&containerState{
		status:    idlewatcher.ContainerStatusRunning,
		ready:     false,
		startedAt: now,
	})
	w.l.Debug().Time("started_at", now).Msg("container starting")
}

func (w *Watcher) setNapping(status idlewatcher.ContainerStatus) {
	w.clearEventHistory() // Clear events on stop/pause
	w.state.Store(&containerState{
		status:      status,
		ready:       false,
		startedAt:   time.Time{},
		healthTries: 0,
	})
}

func (w *Watcher) setError(err error) {
	w.sendEvent(WakeEventError, "Container error", err)
	w.state.Store(&containerState{
		status:      idlewatcher.ContainerStatusError,
		ready:       false,
		err:         err,
		startedAt:   time.Time{},
		healthTries: 0,
	})
}

// waitForReady waits for the container to become ready or context to be canceled.
// Returns true if ready, false if canceled.
func (w *Watcher) waitForReady(ctx context.Context) bool {
	// Check if already ready
	if w.ready() {
		return true
	}

	// Wait for ready notification or context cancellation
	select {
	case <-w.readyNotifyCh:
		return true
	case <-ctx.Done():
		return false
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
