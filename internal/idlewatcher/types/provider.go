package idlewatcher

import (
	"context"

	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/internal/watcher/events"
	gperr "github.com/yusing/goutils/errs"
)

type Provider interface {
	ContainerPause(ctx context.Context) error
	ContainerUnpause(ctx context.Context) error
	ContainerStart(ctx context.Context) error
	ContainerStop(ctx context.Context, signal types.ContainerSignal, timeout int) error
	ContainerKill(ctx context.Context, signal types.ContainerSignal) error
	ContainerStatus(ctx context.Context) (ContainerStatus, error)
	Watch(ctx context.Context) (eventCh <-chan events.Event, errCh <-chan gperr.Error)
	Close()
}
