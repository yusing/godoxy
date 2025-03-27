package watcher

import (
	"context"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/watcher/events"
)

type Event = events.Event

type Watcher interface {
	Events(ctx context.Context) (<-chan Event, <-chan gperr.Error)
}
