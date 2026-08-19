package idlewatcher

import (
	"fmt"
	"io"

	gevents "github.com/yusing/goutils/events"
	strutils "github.com/yusing/goutils/strings"
)

type WakeEvent struct {
	Message string `json:"message"`
	Error   string `json:"error,omitempty"`
}

type WakeEventType string

const (
	WakeEventStarting      WakeEventType = "starting"
	WakeEventWakingDep     WakeEventType = "waking_dep"
	WakeEventDepReady      WakeEventType = "dep_ready"
	WakeEventContainerWoke WakeEventType = "container_woke"
	WakeEventWaitingReady  WakeEventType = "waiting_ready"
	WakeEventReady         WakeEventType = "ready"
	WakeEventError         WakeEventType = "error"
)

const (
	IdleEventCategory            = "idle_event"
	IdleEventActionStarting      = string(WakeEventStarting)
	IdleEventActionWakingDep     = string(WakeEventWakingDep)
	IdleEventActionDepReady      = string(WakeEventDepReady)
	IdleEventActionContainerWoke = string(WakeEventContainerWoke)
	IdleEventActionWaitingReady  = string(WakeEventWaitingReady)
	IdleEventActionReady         = string(WakeEventReady)
	IdleEventActionError         = string(WakeEventError)
	IdleEventActionNapping       = "napping"
)

type IdleActivityEvent struct {
	Container string `json:"container"`
	Route     string `json:"route,omitempty"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

func writeSSE(w io.Writer, v any) error {
	data, err := strutils.MarshalJSON(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func (w *Watcher) newWakeEvent(message string, err error) *WakeEvent {
	event := &WakeEvent{
		Message: message,
	}
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func (e *WakeEvent) WriteSSE(w io.Writer) error {
	return writeSSE(w, e)
}

func (w *Watcher) clearEventHistory() {
	w.eventsMu.Lock()
	defer w.eventsMu.Unlock()

	w.events.Clear()
}

func (w *Watcher) clearTerminalEventHistory() {
	w.eventsMu.Lock()
	defer w.eventsMu.Unlock()

	events := w.events.Get()
	if len(events) != 0 && events[len(events)-1].Action == string(WakeEventError) {
		w.events.Clear()
	}
}

func (w *Watcher) sendEvent(eventType WakeEventType, message string, err error) {
	// NOTE: events will be cleared on stop/pause
	wakeEvent := w.newWakeEvent(message, err)

	w.l.Debug().Str("event", string(eventType)).Str("message", message).Err(err).Msg("sending event")

	level := gevents.LevelInfo
	if eventType == WakeEventError {
		level = gevents.LevelError
	}

	w.eventsMu.Lock()
	defer w.eventsMu.Unlock()

	w.events.Add(gevents.NewEvent(
		level,
		w.cfg.ContainerName(),
		string(eventType),
		wakeEvent,
	))
	w.emitIdleActivity(level, string(eventType), message, err)
}

func (w *Watcher) emitIdleActivity(level gevents.Level, action, message string, err error) {
	if w.task == nil {
		return
	}
	if w.task.Context().Err() != nil {
		return
	}
	history := gevents.FromCtx(w.task.Context())
	if history == nil {
		return
	}

	if err == nil && w.lastIdleAction.Load() == action {
		return
	}
	w.lastIdleAction.Store(action)

	data := &IdleActivityEvent{
		Container: w.cfg.ContainerName(),
		Message:   message,
	}
	if state := w.state.Load(); state != nil && state.status != "" {
		data.Status = string(state.status)
	}
	if w.route != nil {
		data.Route = w.route.Name()
	}
	if err != nil {
		data.Error = err.Error()
	}
	history.Add(gevents.NewEvent(level, IdleEventCategory, action, data))
}
