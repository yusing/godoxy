package idlewatcher

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/yusing/go-proxy/internal/net/types"
)

// Setup implements types.Stream.
func (w *Watcher) Addr() net.Addr {
	return w.stream.Addr()
}

// Setup implements types.Stream.
func (w *Watcher) Setup() error {
	return w.stream.Setup()
}

// Accept implements types.Stream.
func (w *Watcher) Accept() (conn types.StreamConn, err error) {
	conn, err = w.stream.Accept()
	if err != nil {
		return
	}
	if wakeErr := w.wakeFromStream(); wakeErr != nil {
		w.WakeError(wakeErr)
	}
	return
}

// Handle implements types.Stream.
func (w *Watcher) Handle(conn types.StreamConn) error {
	if err := w.wakeFromStream(); err != nil {
		return err
	}
	return w.stream.Handle(conn)
}

// Close implements types.Stream.
func (w *Watcher) Close() error {
	return w.stream.Close()
}

func (w *Watcher) wakeFromStream() error {
	w.resetIdleTimer()

	// pass through if container is already ready
	if w.ready() {
		return nil
	}

	w.WakeDebug().Msg("wake signal received")
	wakeErr := w.wakeIfStopped()
	if wakeErr != nil {
		wakeErr = fmt.Errorf("%s failed: %w", w.String(), wakeErr)
		w.WakeError(wakeErr)
		return wakeErr
	}

	ctx, cancel := context.WithTimeoutCause(w.task.Context(), w.Config().WakeTimeout, errors.New("wake timeout"))
	defer cancel()

	for {
		select {
		case <-w.task.Context().Done():
			cause := w.task.FinishCause()
			w.WakeDebug().Str("cause", cause.Error()).Msg("canceled")
			return cause
		case <-ctx.Done():
			cause := context.Cause(ctx)
			w.WakeDebug().Str("cause", cause.Error()).Msg("timeout")
			return cause
		default:
		}

		if ready, err := w.checkUpdateState(); err != nil {
			return err
		} else if ready {
			w.resetIdleTimer()
			w.Debug().Msg("container is ready, passing through to " + w.hc.URL().String())
			return nil
		}

		// retry until the container is ready or timeout
		time.Sleep(idleWakerCheckInterval)
	}
}
