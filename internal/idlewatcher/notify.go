package idlewatcher

import (
	"time"

	"github.com/rs/zerolog"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/notif"
	strutils "github.com/yusing/goutils/strings"
)

// notifyTransition reports sleep/wake edges observed by the state setters.
func (w *Watcher) notifyTransition(status idlewatcher.ContainerStatus) {
	if w.notify == nil || w.cfg.IdleTimeout == neverTick || !w.cfg.Notify.Wants() {
		return
	}
	name := w.cfg.ContainerName()
	if w.route != nil {
		name = w.route.Name()
	}

	title := "⏰ " + name + " is waking up ⏰"
	if status != idlewatcher.ContainerStatusRunning {
		title = "💤 " + name + " went to sleep 💤"
		if status == idlewatcher.ContainerStatusPaused {
			title = "💤 " + name + " was paused 💤"
		}
	}

	fields := notif.FieldsBody{
		{Name: "Route", Value: name},
		{Name: "Container", Value: w.cfg.ContainerName()},
		{Name: "Time", Value: strutils.FormatTime(time.Now())},
	}
	if status != idlewatcher.ContainerStatusRunning {
		fields.Add("Status", string(status))
		fields.Add("Idle Timeout", strutils.FormatDuration(w.cfg.IdleTimeout))
	}

	w.notify(&notif.LogMessage{
		Level: zerolog.InfoLevel,
		Title: title,
		Body:  fields,
		Color: notif.ColorInfo,
		To:    w.cfg.Notify.To,
	})
}
