package idlewatcher

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/yusing/goutils/events"
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

func writeSSE(w io.Writer, v any) error {
	data, err := json.Marshal(v)
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
	w.events.Clear()
}

func (w *Watcher) sendEvent(eventType WakeEventType, message string, err error) {
	// NOTE: events will be cleared on stop/pause
	wakeEvent := w.newWakeEvent(message, err)

	w.l.Debug().Str("event", string(eventType)).Str("message", message).Err(err).Msg("sending event")

	level := events.LevelInfo
	if eventType == WakeEventError {
		level = events.LevelError
	}

	w.events.Add(events.NewEvent(
		level,
		w.cfg.ContainerName(),
		string(eventType),
		wakeEvent,
	))
}
