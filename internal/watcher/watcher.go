package watcher

import (
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/task"
)

type Event = watcherEvents.Event

type Stream struct {
	Events <-chan Event
	Errors <-chan error
	// Ready yields exactly one initialization result. A nil result means the
	// event source is ready; a non-nil result means initialization failed.
	Ready <-chan error
}

type Watcher interface {
	Watch(parent task.Parent) Stream
}

var ready = func() <-chan error {
	ready := make(chan error)
	close(ready)
	return ready
}()

func Ready() <-chan error {
	return ready
}
