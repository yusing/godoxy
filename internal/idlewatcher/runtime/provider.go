package runtime

import (
	"context"

	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

type Provider interface {
	ContainerPause(ctx context.Context) error
	ContainerUnpause(ctx context.Context) error
	ContainerStart(ctx context.Context) error
	ContainerStop(ctx context.Context, signal ContainerSignal, timeout int) error
	ContainerKill(ctx context.Context, signal ContainerSignal) error
	ContainerStatus(ctx context.Context) (ContainerStatus, error)
	Watch(ctx context.Context) (eventCh <-chan watcherEvents.Event, errCh <-chan error)
	Close()
}
