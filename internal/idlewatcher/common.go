package idlewatcher

import (
	"context"
)

func (w *Watcher) canceled(reqCtx context.Context) bool {
	select {
	case <-reqCtx.Done():
		w.l.Debug().AnErr("cause", context.Cause(reqCtx)).Msg("wake canceled")
		return true
	default:
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
