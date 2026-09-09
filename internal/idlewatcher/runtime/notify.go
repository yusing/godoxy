package runtime

import (
	"errors"
	"slices"
	"strings"

	gperr "github.com/yusing/goutils/errs"
)

// IdlewatcherNotifyEvent is a sleep/wake lifecycle transition that can raise a
// notification through the providers configured under `providers.notification`.
type IdlewatcherNotifyEvent string // @name IdlewatcherNotifyEvent

const (
	NotifyEventSleep       IdlewatcherNotifyEvent = "sleep"        // container stopped or paused
	NotifyEventWake        IdlewatcherNotifyEvent = "wake"         // container started waking up
	NotifyEventReady       IdlewatcherNotifyEvent = "ready"        // container finished waking and is serving
	NotifyEventError       IdlewatcherNotifyEvent = "error"        // container failed to wake
	NotifyEventSleepFailed IdlewatcherNotifyEvent = "sleep_failed" // container failed to stop at idle timeout

	// NotifyEventAll selects every event.
	NotifyEventAll IdlewatcherNotifyEvent = "all"
)

// IdlewatcherNotifyConfig opts a route's idlewatcher into sleep/wake
// notifications. The zero value is disabled.
type IdlewatcherNotifyConfig struct {
	// Enabled is nil by default, meaning: inherit `defaults.idlewatcher.notify`,
	// then fall back to len(To) > 0. Set it explicitly to override the global
	// default in either direction.
	Enabled *bool `json:"enabled,omitzero"`
	// To lists the `providers.notification` names to send to. Empty means every
	// configured provider.
	To []string `json:"to,omitempty"`
	// Events lists the transitions to notify on. Empty means the default set,
	// see NotifyEventsDefault. "all" selects every event.
	Events []IdlewatcherNotifyEvent `json:"events,omitempty"`

	enabled   bool
	eventMask uint8
} // @name IdlewatcherNotifyConfig

// IdlewatcherDefaults holds the `defaults.idlewatcher` config section.
//
// It is deliberately narrow rather than embedding IdlewatcherConfigBase: a
// global idle_timeout default would silently satisfy Route.UseIdleWatcher for
// every container-backed route.
type IdlewatcherDefaults struct {
	Notify IdlewatcherNotifyConfig `json:"notify"`
} // @name IdlewatcherDefaults

var ErrInvalidNotifyEvent = errors.New("invalid idlewatcher notify event")

// NotifyEventsDefault is the event set used when `events` is not configured.
var NotifyEventsDefault = []IdlewatcherNotifyEvent{NotifyEventSleep, NotifyEventWake}

var notifyEventBits = map[IdlewatcherNotifyEvent]uint8{
	NotifyEventSleep:       1 << 0,
	NotifyEventWake:        1 << 1,
	NotifyEventReady:       1 << 2,
	NotifyEventError:       1 << 3,
	NotifyEventSleepFailed: 1 << 4,
}

const notifyEventMaskAll = uint8(1<<5 - 1)

// Validate implements serialization.CustomValidator.
//
// It runs at deserialization time for both YAML routes and Docker labels,
// independently of whether the enclosing IdlewatcherConfig has a positive
// idle_timeout.
func (c *IdlewatcherNotifyConfig) Validate() error {
	for i, event := range c.Events {
		normalized := IdlewatcherNotifyEvent(strings.ToLower(strings.TrimSpace(string(event))))
		if _, ok := notifyEventBits[normalized]; !ok && normalized != NotifyEventAll {
			return gperr.PrependSubject(ErrInvalidNotifyEvent, string(event)).
				Withf("expect one of: %s, %s, %s, %s, %s, %s",
					NotifyEventAll, NotifyEventSleep, NotifyEventWake,
					NotifyEventReady, NotifyEventError, NotifyEventSleepFailed)
		}
		c.Events[i] = normalized
	}
	c.resolve()
	return nil
}

// ApplyDefaults fills unset fields from `defaults.idlewatcher.notify` and
// recomputes the effective enabled flag and event mask. It is idempotent.
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

// resolve recomputes the unexported effective state. It is idempotent.
func (c *IdlewatcherNotifyConfig) resolve() {
	if c.Enabled != nil {
		c.enabled = *c.Enabled
	} else {
		// Same opt-in ergonomic as acl.notify: naming providers enables it.
		c.enabled = len(c.To) > 0
	}

	// NOTE: Events is deliberately left alone. See ApplyDefaults.
	events := c.Events
	if len(events) == 0 {
		events = NotifyEventsDefault
	}
	c.eventMask = eventMask(events)
}

func eventMask(events []IdlewatcherNotifyEvent) uint8 {
	var mask uint8
	for _, event := range events {
		if event == NotifyEventAll {
			return notifyEventMaskAll
		}
		mask |= notifyEventBits[event]
	}
	return mask
}

// Wants reports whether event should raise a notification.
func (c *IdlewatcherNotifyConfig) Wants(event IdlewatcherNotifyEvent) bool {
	return c != nil && c.enabled && c.eventMask&notifyEventBits[event] != 0
}
