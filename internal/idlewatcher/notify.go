package idlewatcher

import (
	"time"

	"github.com/rs/zerolog"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/notif"
	strutils "github.com/yusing/goutils/strings"
)

// notifyPhase is the lifecycle phase the last notification decision was made
// for. Sleep/wake notifications are edge triggered on this value.
//
// It is deliberately not derived from lastIdleAction: sendEvent overwrites that
// field for every wake sub-event (waking_dep, container_woke, waiting_ready),
// so by the time setStarting runs on the request path it no longer reflects the
// last lifecycle transition.
type notifyPhase uint32

const (
	notifyPhaseAsleep notifyPhase = iota
	notifyPhaseAwake
)

// initialNotifyPhase seeds the edge detector from the container status observed
// when the watcher is created, so that GoDoxy starting up next to an already
// running container does not report a wake that happened before it was
// watching.
func initialNotifyPhase(status idlewatcher.ContainerStatus) notifyPhase {
	if status == idlewatcher.ContainerStatusRunning {
		return notifyPhaseAwake
	}
	return notifyPhaseAsleep
}

// notifyTransition reports a sleep/wake change through the configured
// notification providers.
func (w *Watcher) notifyTransition(phase notifyPhase, event idlewatcher.IdlewatcherNotifyEvent, status string) {
	// Dependency-only watchers inherit their parent's IdlewatcherConfigBase and
	// are started and stopped as a side effect of the parent, so reporting them
	// separately would duplicate every notification.
	if w.notify == nil || w.cfg.IdleTimeout == neverTick || !w.cfg.Notify.Wants() {
		return
	}
	if prev := notifyPhase(w.notifyPhase.Swap(uint32(phase))); prev == phase {
		return
	}
	w.notify(w.buildNotification(event, status))
}

// displayName prefers the route name, which is what the user configured and
// what the dashboard shows. Dependency and test watchers may have no route.
func (w *Watcher) displayName() string {
	if w.route != nil {
		return w.route.Name()
	}
	return w.cfg.ContainerName()
}

func (w *Watcher) buildNotification(event idlewatcher.IdlewatcherNotifyEvent, status string) *notif.LogMessage {
	name := w.displayName()

	title := "⏰ " + name + " is waking up ⏰"
	if event == idlewatcher.NotifyEventSleep {
		title = "💤 " + name + " went to sleep 💤"
		if status == string(idlewatcher.ContainerStatusPaused) {
			title = "💤 " + name + " was paused 💤"
		}
	}

	// NOTE: FieldsBody is a slice, so it must be complete before it is assigned
	// to msg.Body; an Add after the assignment would not be visible on msg.
	fields := notif.FieldsBody{
		{Name: "Route", Value: name},
		{Name: "Container", Value: w.cfg.ContainerName()},
		{Name: "Time", Value: strutils.FormatTime(time.Now())},
	}
	if event == idlewatcher.NotifyEventSleep {
		fields.Add("Status", status)
		fields.Add("Idle Timeout", strutils.FormatDuration(w.cfg.IdleTimeout))
	}

	return &notif.LogMessage{
		Level: zerolog.InfoLevel,
		Title: title,
		Body:  fields,
		Color: notif.ColorInfo,
		To:    w.cfg.Notify.To,
	}
}
