package watcher

import (
	"context"

	gperr "github.com/yusing/goutils/errs"
)

type fileWatcher struct {
	relPath string
	eventCh chan Event
	errCh   chan gperr.Error
}

func (fw *fileWatcher) Events(ctx context.Context) (<-chan Event, <-chan gperr.Error) {
	return fw.eventCh, fw.errCh
}
