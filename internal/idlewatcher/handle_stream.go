package idlewatcher

import (
	"context"
	"errors"
	"net"
	"time"

	gpnet "github.com/yusing/go-proxy/internal/net/types"
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
func (w *Watcher) Accept() (conn gpnet.StreamConn, err error) {
	conn, err = w.stream.Accept()
	if err != nil {
		return
	}
	if wakeErr := w.wakeFromStream(); wakeErr != nil {
		w.l.Err(wakeErr).Msg("error waking container")
	}
	return
}

// Handle implements types.Stream.
func (w *Watcher) Handle(conn gpnet.StreamConn) error {
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

	w.l.Debug().Msg("wake signal received")
	err := w.wakeIfStopped()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeoutCause(w.task.Context(), w.cfg.WakeTimeout, errors.New("wake timeout"))
	defer cancel()

	var ready bool

	for {
		if w.cancelled(ctx) {
			return context.Cause(ctx)
		}

		w, ready, err = checkUpdateState(w.Key())
		if err != nil {
			return err
		}
		if ready {
			w.resetIdleTimer()
			w.l.Debug().Stringer("url", w.hc.URL()).Msg("container is ready, passing through")
			return nil
		}

		// retry until the container is ready or timeout
		time.Sleep(idleWakerCheckInterval)
	}
}
