package runtime

import (
	"errors"
	"slices"
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

// IdlewatcherNotifyEvent is a sleep/wake transition that can raise a
// notification through the providers under `providers.notification`.
type IdlewatcherNotifyEvent string // @name IdlewatcherNotifyEvent

const (
	NotifyEventSleep       IdlewatcherNotifyEvent = "sleep"        // container stopped or paused
	NotifyEventWake        IdlewatcherNotifyEvent = "wake"         // container started waking up
	NotifyEventReady       IdlewatcherNotifyEvent = "ready"        // container finished waking and is serving
	NotifyEventError       IdlewatcherNotifyEvent = "error"        // container failed to wake
	NotifyEventSleepFailed IdlewatcherNotifyEvent = "sleep_failed" // container failed to stop at idle timeout
	NotifyEventAll         IdlewatcherNotifyEvent = "all"          // every event
)

var (
	ErrInvalidNotifyEvent = errors.New("invalid idlewatcher notify event")

	// notifyEvents doubles as the bit order behind eventMask.
	notifyEvents = []IdlewatcherNotifyEvent{
		NotifyEventSleep, NotifyEventWake, NotifyEventReady, NotifyEventError, NotifyEventSleepFailed,
	}
	// NotifyEventsDefault applies when `events` is not configured.
	NotifyEventsDefault = []IdlewatcherNotifyEvent{NotifyEventSleep, NotifyEventWake}
)

// IdlewatcherNotifyConfig opts a route's idlewatcher into sleep/wake
// notifications. The zero value is disabled.
type IdlewatcherNotifyConfig struct {
	// Opt in or out explicitly. Unset inherits `defaults.idlewatcher.notify`,
	// then falls back to len(To) > 0.
	Enabled *bool `json:"enabled,omitzero"`
	// `providers.notification` names to send to. Empty means all of them.
	To []string `json:"to,omitempty"`
	// Transitions to notify on. Empty means NotifyEventsDefault.
	Events []IdlewatcherNotifyEvent `json:"events,omitempty"`

	enabled   bool
	eventMask uint8
} // @name IdlewatcherNotifyConfig

// IdlewatcherDefaults holds the `defaults.idlewatcher` section. It is
// deliberately narrow: a global idle_timeout would silently satisfy
// Route.UseIdleWatcher for every container-backed route.
type IdlewatcherDefaults struct {
	Notify IdlewatcherNotifyConfig `json:"notify"`
} // @name IdlewatcherDefaults

// Validate implements serialization.CustomValidator. It runs at deserialization
// time for YAML routes and Docker labels alike.
func (c *IdlewatcherNotifyConfig) Validate() error {
	for i, event := range c.Events {
		normalized := IdlewatcherNotifyEvent(strings.ToLower(strings.TrimSpace(string(event))))
		if normalized != NotifyEventAll && !slices.Contains(notifyEvents, normalized) {
			return gperr.PrependSubject(ErrInvalidNotifyEvent, string(event)).
				Withf("expect one of: all, sleep, wake, ready, error, sleep_failed")
		}
		c.Events[i] = normalized
	}
	c.resolve()
	return nil
}

// ApplyDefaults fills unset fields from `defaults.idlewatcher.notify`. It is
// idempotent.
func (c *IdlewatcherNotifyConfig) ApplyDefaults(defaults IdlewatcherNotifyConfig) {
	if c.Enabled == nil {
		c.Enabled = defaults.Enabled
	}
	if len(c.To) == 0 {
		c.To = slices.Clone(defaults.To)
	}
	if len(c.Events) == 0 {
		c.Events = slices.Clone(defaults.Events)
	}
	// Materialize the built-in set only here, once the globals have had their
	// chance. resolve must not do it: it runs at deserialization time, and a
	// populated Events would make the inheritance above a no-op.
	if len(c.Events) == 0 {
		c.Events = slices.Clone(NotifyEventsDefault)
	}
	c.resolve()
}

// resolve recomputes the effective state. Idempotent, and must not touch Events.
func (c *IdlewatcherNotifyConfig) resolve() {
	// Same opt-in ergonomic as acl.notify: naming providers enables it.
	c.enabled = len(c.To) > 0
	if c.Enabled != nil {
		c.enabled = *c.Enabled
	}

	events := c.Events
	if len(events) == 0 {
		events = NotifyEventsDefault
	}
	c.eventMask = 0
	for _, event := range events {
		if event == NotifyEventAll {
			c.eventMask = 1<<len(notifyEvents) - 1
			break
		}
		c.eventMask |= eventBit(event)
	}
}

// Wants reports whether event should raise a notification.
func (c *IdlewatcherNotifyConfig) Wants(event IdlewatcherNotifyEvent) bool {
	return c != nil && c.enabled && c.eventMask&eventBit(event) != 0
}

func eventBit(event IdlewatcherNotifyEvent) uint8 {
	if i := slices.Index(notifyEvents, event); i >= 0 {
		return 1 << i
	}
	return 0
}
