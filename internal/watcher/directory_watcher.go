package watcher

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

const watcherChannelCapacity = 10

type DirWatcher struct {
	zerolog.Logger

	dir string
	w   *fsnotify.Watcher

	fwMap map[string]*fileWatcher
	mu    sync.Mutex

	eventCh          chan Event
	errCh            chan error
	eventSubscribers atomic.Int64

	task *task.Task
}

// NewDirectoryWatcher returns a DirWatcher instance.
//
// The DirWatcher watches the given directory for file system events.
// Currently, only events on files directly in the given directory are watched, not
// recursively.
//
// Note that the returned DirWatcher is not ready to use until the goroutine
// started by NewDirectoryWatcher has finished.
func NewDirectoryWatcher(parent task.Parent, dirPath string) *DirWatcher {
	//! subdirectories are not watched
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Panic().Err(err).Msg("unable to create fs watcher")
	}
	if err = w.Add(dirPath); err != nil {
		log.Panic().Err(err).Msg("unable to create fs watcher")
	}
	helper := &DirWatcher{
		Logger: log.With().
			Str("type", "dir").
			Str("path", dirPath).
			Logger(),
		dir:     dirPath,
		w:       w,
		fwMap:   make(map[string]*fileWatcher),
		eventCh: make(chan Event, watcherChannelCapacity),
		errCh:   make(chan error, watcherChannelCapacity),
		task:    parent.Subtask("dir_watcher("+dirPath+")", true),
	}
	go helper.start()
	return helper
}

var _ Watcher = (*DirWatcher)(nil)

// Watch implements the Watcher interface.
func (h *DirWatcher) Watch(parent task.Parent) Stream {
	h.eventSubscribers.Add(1)
	parent.OnCancel("remove directory watcher subscriber", func() {
		h.eventSubscribers.Add(-1)
	})
	return Stream{Events: h.eventCh, Errors: h.errCh, Ready: Ready()}
}

func (h *DirWatcher) Add(relPath string) Watcher {
	h.mu.Lock()
	defer h.mu.Unlock()

	// check if the watcher already exists
	s, ok := h.fwMap[relPath]
	if ok {
		return s
	}
	s = &fileWatcher{
		relPath: relPath,
		eventCh: make(chan Event, watcherChannelCapacity),
		errCh:   make(chan error, watcherChannelCapacity),
	}
	h.fwMap[relPath] = s
	return s
}

func (h *DirWatcher) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.w.Close()
	close(h.eventCh)
	close(h.errCh)
	for _, fw := range h.fwMap {
		close(fw.eventCh)
		close(fw.errCh)
	}
	h.task.Finish(nil)
}

func (h *DirWatcher) start() {
	defer h.cleanup()

	for {
		select {
		case <-h.task.Context().Done():
			return
		case fsEvent, ok := <-h.w.Events:
			if !ok {
				return
			}
			// retrieve the watcher
			relPath := strings.TrimPrefix(fsEvent.Name, h.dir)
			relPath = strings.TrimPrefix(relPath, "/")

			if len(relPath) > 0 && relPath[0] == '.' { // hidden file
				continue
			}

			msg := Event{
				Type:      watcherEvents.EventTypeFile,
				ActorName: relPath,
			}
			switch {
			case fsEvent.Has(fsnotify.Write):
				msg.Action = watcherEvents.ActionFileWritten
			case fsEvent.Has(fsnotify.Create):
				msg.Action = watcherEvents.ActionFileCreated
			case fsEvent.Has(fsnotify.Remove):
				msg.Action = watcherEvents.ActionFileDeleted
			case fsEvent.Has(fsnotify.Rename):
				msg.Action = watcherEvents.ActionFileRenamed
			default: // ignore other events
				continue
			}

			if !h.dispatchEvent(relPath, msg) {
				return
			}
		case err, ok := <-h.w.Errors:
			if !ok || errors.Is(err, fsnotify.ErrClosed) {
				// closed manually?
				return
			}
			if !h.dispatchError(err) {
				return
			}
		}
	}
}

func (h *DirWatcher) dispatchEvent(relPath string, msg Event) bool {
	if h.eventSubscribers.Load() > 0 {
		select {
		case h.eventCh <- msg:
			h.Debug().Msg("sent event to directory watcher")
		case <-h.task.Context().Done():
			return false
		}
	}

	h.mu.Lock()
	w, ok := h.fwMap[relPath]
	h.mu.Unlock()
	if !ok {
		h.Debug().Msg("file watcher not found: " + relPath)
		return true
	}

	select {
	case w.eventCh <- msg:
		h.Debug().Msg("sent event to file watcher " + relPath)
	case <-h.task.Context().Done():
		return false
	}
	return true
}

func (h *DirWatcher) dispatchError(err error) bool {
	if h.eventSubscribers.Load() == 0 {
		return true
	}

	select {
	case h.errCh <- err:
	case <-h.task.Context().Done():
		return false
	}
	return true
}
