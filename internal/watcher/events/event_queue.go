package events

import (
	"runtime/debug"
	"time"

	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/task"
)

type (
	EventQueue struct {
		task          *task.Task
		queue         []Event
		ticker        *time.Ticker
		flushInterval time.Duration
		onFlush       OnFlushFunc
		onError       OnErrorFunc
	}
	OnFlushFunc = func(events []Event)
	OnErrorFunc = func(err gperr.Error)
)

const eventQueueCapacity = 10

// NewEventQueue returns a new EventQueue with the given
// queueTask, flushInterval, onFlush and onError.
//
// The returned EventQueue will start a goroutine to flush events in the queue
// when the flushInterval is reached.
//
// The onFlush function is called when the flushInterval is reached and the queue is not empty,
//
// The onError function is called when an error received from the errCh,
// or panic occurs in the onFlush function. Panic will cause a E.ErrPanicRecv error.
//
// flushTask.Finish must be called after the flush is done,
// but the onFlush function can return earlier (e.g. run in another goroutine).
//
// If task is canceled before the flushInterval is reached, the events in queue will be discarded.
func NewEventQueue(queueTask *task.Task, flushInterval time.Duration, onFlush OnFlushFunc, onError OnErrorFunc) *EventQueue {
	return &EventQueue{
		task:          queueTask,
		queue:         make([]Event, 0, eventQueueCapacity),
		ticker:        time.NewTicker(flushInterval),
		flushInterval: flushInterval,
		onFlush:       onFlush,
		onError:       onError,
	}
}

func (e *EventQueue) Start(eventCh <-chan Event, errCh <-chan gperr.Error) {
	origOnFlush := e.onFlush
	// recover panic in onFlush when in production mode
	e.onFlush = func(events []Event) {
		defer func() {
			if err := recover(); err != nil {
				e.onError(gperr.New("recovered panic in onFlush").
					Withf("%v", err).
					Subject(e.task.Name()))
				if common.IsDebug {
					panic(string(debug.Stack()))
				}
			}
		}()
		origOnFlush(events)
	}

	go func() {
		defer e.ticker.Stop()
		defer e.task.Finish(nil)

		for {
			select {
			case <-e.task.Context().Done():
				return
			case <-e.ticker.C:
				if len(e.queue) > 0 {
					// clone -> clear -> flush
					queue := make([]Event, len(e.queue))
					copy(queue, e.queue)

					e.queue = e.queue[:0]

					e.onFlush(queue)
				}
				e.ticker.Reset(e.flushInterval)
			case event, ok := <-eventCh:
				if !ok {
					return
				}
				e.queue = append(e.queue, event)
			case err, ok := <-errCh:
				if !ok {
					return
				}
				if err != nil {
					e.onError(err)
				}
			}
		}
	}()
}
