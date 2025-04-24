package idlewatcher

import "context"

func (w *Watcher) cancelled(reqCtx context.Context) bool {
	select {
	case <-reqCtx.Done():
		w.l.Debug().AnErr("cause", context.Cause(reqCtx)).Msg("wake canceled")
		return true
	default:
		return false
	}
}
