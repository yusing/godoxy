package watcher

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type DirWatcher struct {
	zerolog.Logger

	dir string
	w   *fsnotify.Watcher

	fwMap map[string]*fileWatcher
	mu    sync.Mutex

	eventCh chan Event
	errCh   chan gperr.Error

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
		logging.Panic().Err(err).Msg("unable to create fs watcher")
	}
	if err = w.Add(dirPath); err != nil {
		logging.Panic().Err(err).Msg("unable to create fs watcher")
	}
	helper := &DirWatcher{
		Logger: logging.With().
			Str("type", "dir").
			Str("path", dirPath).
			Logger(),
		dir:     dirPath,
		w:       w,
		fwMap:   make(map[string]*fileWatcher),
		eventCh: make(chan Event),
		errCh:   make(chan gperr.Error),
		task:    parent.Subtask("dir_watcher(" + dirPath + ")"),
	}
	go helper.start()
	return helper
}

func (h *DirWatcher) Events(_ context.Context) (<-chan Event, <-chan gperr.Error) {
	return h.eventCh, h.errCh
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
		eventCh: make(chan Event),
		errCh:   make(chan gperr.Error),
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

			if len(relPath) > 0 && relPath[0] == '.' { // hideden file
				continue
			}

			msg := Event{
				Type:      events.EventTypeFile,
				ActorName: relPath,
			}
			switch {
			case fsEvent.Has(fsnotify.Write):
				msg.Action = events.ActionFileWritten
			case fsEvent.Has(fsnotify.Create):
				msg.Action = events.ActionFileCreated
			case fsEvent.Has(fsnotify.Remove):
				msg.Action = events.ActionFileDeleted
			case fsEvent.Has(fsnotify.Rename):
				msg.Action = events.ActionFileRenamed
			default: // ignore other events
				continue
			}

			// send event to directory watcher
			select {
			case h.eventCh <- msg:
				h.Debug().Msg("sent event to directory watcher")
			default:
				h.Debug().Msg("failed to send event to directory watcher")
			}

			// send event to file watcher too
			h.mu.Lock()
			w, ok := h.fwMap[relPath]
			h.mu.Unlock()
			if ok {
				select {
				case w.eventCh <- msg:
					h.Debug().Msg("sent event to file watcher " + relPath)
				default:
					h.Debug().Msg("failed to send event to file watcher " + relPath)
				}
			} else {
				h.Debug().Msg("file watcher not found: " + relPath)
			}
		case err := <-h.w.Errors:
			if errors.Is(err, fsnotify.ErrClosed) {
				// closed manually?
				return
			}
			select {
			case h.errCh <- gperr.Wrap(err):
			default:
			}
		}
	}
}
