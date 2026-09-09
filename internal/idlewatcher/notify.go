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
	notifyPhaseWaking
	notifyPhaseAwake
	notifyPhaseErrored
)

// initialNotifyPhase seeds the edge detector from the container status observed
// when the watcher is created, so that GoDoxy starting up next to an already
// running container does not report a wake that happened before it was
// watching.
func initialNotifyPhase(status idlewatcher.ContainerStatus) notifyPhase {
	if status == idlewatcher.ContainerStatusRunning {
		return notifyPhaseWaking
	}
	return notifyPhaseAsleep
}

// notifyTransition reports a sleep/wake state change through the configured
// notification providers.
//
// The phase is recorded unconditionally, before the event filter, so the edge
// detector stays accurate even for events this route has filtered out.
func (w *Watcher) notifyTransition(phase notifyPhase, event idlewatcher.IdlewatcherNotifyEvent, detail string, err error) {
	if !w.canNotify() {
		return
	}
	if prev := notifyPhase(w.notifyPhase.Swap(uint32(phase))); prev == phase {
		return
	}
	if !w.cfg.Notify.Wants(event) {
		return
	}
	w.notify(w.buildNotification(event, detail, err))
}

// notifyOneShot reports a failure that is not a state transition, so it skips
// the phase edge check.
func (w *Watcher) notifyOneShot(event idlewatcher.IdlewatcherNotifyEvent, detail string, err error) {
	if !w.canNotify() || !w.cfg.Notify.Wants(event) {
		return
	}
	w.notify(w.buildNotification(event, detail, err))
}

func (w *Watcher) canNotify() bool {
	if w.notify == nil {
		return false
	}
	// Dependency-only watchers inherit their parent's IdlewatcherConfigBase and
	// are started and stopped as a side effect of the parent, so reporting them
	// separately would duplicate every notification.
	return w.cfg.IdleTimeout != neverTick
}

// displayName prefers the route name, which is what the user configured and
// what the dashboard shows. Dependency and test watchers may have no route.
func (w *Watcher) displayName() string {
	if w.route != nil {
		return w.route.Name()
	}
	return w.cfg.ContainerName()
}

func (w *Watcher) buildNotification(event idlewatcher.IdlewatcherNotifyEvent, detail string, err error) *notif.LogMessage {
	name := w.displayName()

	// NOTE: FieldsBody is a slice, so it must be complete before it is assigned
	// to msg.Body; an Add after the assignment would not be visible on msg.
	fields := notif.FieldsBody{
		{Name: "Route", Value: name},
		{Name: "Container", Value: w.cfg.ContainerName()},
		{Name: "Time", Value: strutils.FormatTime(time.Now())},
	}

	var title string
	level := zerolog.InfoLevel
	color := notif.ColorInfo

	switch event {
	case idlewatcher.NotifyEventSleep:
		title = "💤 " + name + " went to sleep 💤"
		if detail == string(idlewatcher.ContainerStatusPaused) {
			title = "💤 " + name + " was paused 💤"
		}
		fields.Add("Status", detail)
		fields.Add("Idle Timeout", strutils.FormatDuration(w.cfg.IdleTimeout))
	case idlewatcher.NotifyEventWake:
		title = "⏰ " + name + " is waking up ⏰"
	case idlewatcher.NotifyEventReady:
		title = "✅ " + name + " is awake ✅"
		color = notif.ColorSuccess
	case idlewatcher.NotifyEventError:
		title = "❌ " + name + " failed to wake ❌"
		level, color = zerolog.WarnLevel, notif.ColorError
	case idlewatcher.NotifyEventSleepFailed:
		title = "❌ " + name + " failed to sleep ❌"
		level, color = zerolog.WarnLevel, notif.ColorError
	}

	if detail != "" && event != idlewatcher.NotifyEventSleep {
		fields.Add("Detail", detail)
	}
	if err != nil {
		fields.Add("Error", err.Error())
	}

	return &notif.LogMessage{
		Level: level,
		Title: title,
		Body:  fields,
		Color: color,
		To:    w.cfg.Notify.To,
	}
}
