package runtime

import "slices"

// IdlewatcherNotifyConfig opts a route's idlewatcher into sleep/wake
// notifications. The zero value is disabled.
type IdlewatcherNotifyConfig struct {
	// Opt in or out explicitly. Unset inherits `defaults.idlewatcher.notify`,
	// then falls back to len(To) > 0.
	Enabled *bool `json:"enabled,omitzero"`
	// `providers.notification` names to send to. Empty means all of them.
	To []string `json:"to,omitempty"`
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
}

// Wants reports whether sleep/wake notifications are enabled.
func (c IdlewatcherNotifyConfig) Wants() bool {
	if c.Enabled != nil {
		return *c.Enabled
	}
	return len(c.To) > 0
}
