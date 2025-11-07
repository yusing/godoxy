package idlewatcher

import (
	"fmt"
	"io"
	"time"

	"github.com/bytedance/sonic"
)

type WakeEvent struct {
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
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

func (w *Watcher) newWakeEvent(eventType WakeEventType, message string, err error) *WakeEvent {
	event := &WakeEvent{
		Type:      string(eventType),
		Message:   message,
		Timestamp: time.Now(),
	}
	if err != nil {
		event.Error = err.Error()
	}
	return event
}

func (e *WakeEvent) WriteSSE(w io.Writer) error {
	data, err := sonic.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func (w *Watcher) clearEventHistory() {
	w.eventHistoryMu.Lock()
	w.eventHistory = w.eventHistory[:0]
	w.eventHistoryMu.Unlock()
}

func (w *Watcher) sendEvent(eventType WakeEventType, message string, err error) {
	event := w.newWakeEvent(eventType, message, err)

	w.l.Debug().Str("event", string(eventType)).Str("message", message).Err(err).Msg("sending event")

	// Store event in history
	w.eventHistoryMu.Lock()
	w.eventHistory = append(w.eventHistory, *event)
	w.eventHistoryMu.Unlock()

	// Broadcast to current subscribers
	for ch := range w.eventChs.Range {
		select {
		case ch <- event:
		default:
			// channel full, drop event
		}
	}
}
