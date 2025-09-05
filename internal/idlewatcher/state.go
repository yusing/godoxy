package idlewatcher

import (
	"context"
	"time"

	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
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
	w.state.Store(&containerState{
		status:      status,
		ready:       false,
		startedAt:   time.Time{},
		healthTries: 0,
	})
}

func (w *Watcher) setError(err error) {
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
		return w.ready() // double-check in case of race condition
	case <-ctx.Done():
		return false
	}
}
