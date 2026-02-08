package watcher

import (
	"context"

	"github.com/yusing/godoxy/internal/watcher/events"
)

type Event = events.Event

type Watcher interface {
	Events(ctx context.Context) (<-chan Event, <-chan error)
}
