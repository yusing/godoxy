package runtime

import "slices"

// IdlewatcherNotifyEvent is a sleep/wake transition that raises a notification
// through the providers under `providers.notification`.
type IdlewatcherNotifyEvent string // @name IdlewatcherNotifyEvent

const (
	NotifyEventSleep IdlewatcherNotifyEvent = "sleep" // container stopped or paused
	NotifyEventWake  IdlewatcherNotifyEvent = "wake"  // container started waking up
)

// IdlewatcherNotifyConfig opts a route's idlewatcher into sleep/wake
// notifications. The zero value is disabled.
type IdlewatcherNotifyConfig struct {
	// Opt in or out explicitly. Unset inherits `defaults.idlewatcher.notify`,
	// then falls back to len(To) > 0.
	Enabled *bool `json:"enabled,omitzero"`
	// `providers.notification` names to send to. Empty means all of them.
	To []string `json:"to,omitempty"`

	enabled bool
} // @name IdlewatcherNotifyConfig

// IdlewatcherDefaults holds the `defaults.idlewatcher` section. It is
// deliberately narrow: a global idle_timeout would silently satisfy
// Route.UseIdleWatcher for every container-backed route.
type IdlewatcherDefaults struct {
	Notify IdlewatcherNotifyConfig `json:"notify"`
} // @name IdlewatcherDefaults

// ApplyDefaults fills unset fields from `defaults.idlewatcher.notify`. It is
// idempotent.
func (c *IdlewatcherNotifyConfig) ApplyDefaults(defaults IdlewatcherNotifyConfig) {
	if c.Enabled == nil {
		c.Enabled = defaults.Enabled
	}
	if len(c.To) == 0 {
		c.To = slices.Clone(defaults.To)
	}
	c.resolve()
}

// resolve recomputes the effective enabled flag. Idempotent.
func (c *IdlewatcherNotifyConfig) resolve() {
	// Same opt-in ergonomic as acl.notify: naming providers enables it.
	c.enabled = len(c.To) > 0
	if c.Enabled != nil {
		c.enabled = *c.Enabled
	}
}

// Wants reports whether sleep/wake notifications are enabled.
func (c *IdlewatcherNotifyConfig) Wants() bool {
	return c != nil && c.enabled
}
