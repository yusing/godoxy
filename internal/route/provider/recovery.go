package provider

import (
	"context"
	"time"

	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/godoxy/internal/watcher"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
	"github.com/yusing/goutils/eventqueue"
	"github.com/yusing/goutils/task"
)

func (p *Provider) canRetry() bool {
	return p.t == routing.ProviderTypeDocker || p.t == routing.ProviderTypeAgent
}

func forceReloadEvents() []watcherEvents.Event {
	return []watcherEvents.Event{{Type: watcherEvents.EventTypeDocker, Action: watcherEvents.ActionForceReload}}
}

// Retry delays are bounded, and cancellation always interrupts the backoff.
func waitProviderRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func (p *Provider) handleEvents(parent task.Parent, events []watcherEvents.Event) {
	delay := 3 * time.Second
	for parent.Context().Err() == nil {
		handler := p.newEventHandler()
		handler.Handle(parent, events)
		handler.Log()
		if !p.canRetry() || !p.retryLoad {
			return
		}
		p.Logger().Warn().Dur("retry_in", delay).Msg("provider route load failed; retrying")
		if !waitProviderRetry(parent.Context(), delay) {
			return
		}
		delay = min(delay*2, 30*time.Second)
		events = forceReloadEvents()
	}
}

func (p *Provider) startEventQueue(parent *task.Task, stream watcher.Stream, opts eventqueue.Options[watcherEvents.Event]) {
	if parent.Context().Err() != nil {
		return
	}
	queue := eventqueue.New(parent.Subtask("event_queue", false), opts)
	queue.Start(stream.Events, stream.Errors)
}

func (p *Provider) retryWatcher(parent task.Parent, opts eventqueue.Options[watcherEvents.Event]) {
	recovery := parent.Subtask("watcher_recovery", true)
	go func() {
		defer recovery.Finish(nil)
		delay := 3 * time.Second
		for {
			p.Logger().Warn().Dur("retry_in", delay).Msg("provider watcher unavailable; retrying")
			if !waitProviderRetry(recovery.Context(), delay) {
				return
			}
			// Loading also initializes missing agents before the watcher resolves
			// their Docker clients through the runtime's agent pool.
			p.handleEvents(parent, forceReloadEvents())
			if recovery.Context().Err() != nil {
				return
			}
			watcherTask := parent.Subtask("watcher", false)
			stream := p.watcher.Watch(watcherTask)
			var err error
			if stream.Ready == nil || stream.Events == nil || stream.Errors == nil {
				err = ErrWatcherStreamUnavailable
			} else {
				select {
				case err = <-stream.Ready:
				case <-recovery.Context().Done():
					err = context.Cause(recovery.Context())
				}
			}
			if err != nil {
				watcherTask.FinishAndWait(err)
				p.Logger().Warn().Err(err).Msg("provider watcher retry failed")
				delay = min(delay*2, 30*time.Second)
				continue
			}
			p.startEventQueue(watcherTask, stream, opts)
			p.Logger().Info().Msg("provider watcher recovered")
			return
		}
	}()
}
