package watcher

import "github.com/yusing/goutils/task"

type fileWatcher struct {
	relPath string
	eventCh chan Event
	errCh   chan error
}

var _ Watcher = (*fileWatcher)(nil)

// Watch implements the Watcher interface.
func (fw *fileWatcher) Watch(task.Parent) Stream {
	return Stream{Events: fw.eventCh, Errors: fw.errCh, Ready: Ready()}
}
