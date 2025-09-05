package idlewatcher

import (
	"context"
	"net"

	nettypes "github.com/yusing/go-proxy/internal/net/types"
)

var _ nettypes.Stream = (*Watcher)(nil)

// ListenAndServe implements nettypes.Stream.
func (w *Watcher) ListenAndServe(ctx context.Context, predial, onRead nettypes.HookFunc) {
	w.stream.ListenAndServe(ctx, func(ctx context.Context) error { //nolint:contextcheck
		return w.preDial(ctx, predial)
	}, func(ctx context.Context) error {
		return w.onRead(ctx, onRead)
	})
}

// Close implements nettypes.Stream.
func (w *Watcher) Close() error {
	return w.stream.Close()
}

// LocalAddr implements nettypes.Stream.
func (w *Watcher) LocalAddr() net.Addr {
	return w.stream.LocalAddr()
}

func (w *Watcher) preDial(ctx context.Context, predial nettypes.HookFunc) error {
	if predial != nil {
		if err := predial(ctx); err != nil {
			return err
		}
	}

	return w.wakeFromStream(ctx)
}

func (w *Watcher) onRead(ctx context.Context, onRead nettypes.HookFunc) error {
	w.resetIdleTimer()
	if onRead != nil {
		if err := onRead(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (w *Watcher) wakeFromStream(ctx context.Context) error {
	w.resetIdleTimer()

	// pass through if container is already ready
	if w.ready() {
		return nil
	}

	w.l.Debug().Msg("wake signal received")
	err := w.Wake(ctx)
	if err != nil {
		return err
	}

	// Wait for route to be started
	if !w.waitStarted(ctx) {
		return nil
	}

	// Wait for container to become ready
	if !w.waitForReady(ctx) {
		return nil // canceled or failed
	}

	// Container is ready
	w.resetIdleTimer()
	w.l.Debug().Stringer("url", w.hc.URL()).Msg("container is ready, passing through")
	return nil
}
