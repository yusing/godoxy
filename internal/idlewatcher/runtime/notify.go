package runtime

import "slices"

// IdlewatcherNotifyConfig selects sleep/wake notification providers.
// Omitted targets inherit defaults or use all providers at send time; [] disables.
type IdlewatcherNotifyConfig struct {
	To []string `json:"to,omitzero" extensions:"x-omitempty"`
} // @name IdlewatcherNotifyConfig

// IdlewatcherDefaults cannot set timeouts, which would enable idlewatchers.
type IdlewatcherDefaults struct {
	Notify *IdlewatcherNotifyConfig `json:"notify,omitzero"`
} // @name IdlewatcherDefaults

// ApplyDefaults resolves static targets without capturing the provider list.
func (c *IdlewatcherNotifyConfig) ApplyDefaults(defaults *IdlewatcherNotifyConfig) *IdlewatcherNotifyConfig {
	if c == nil {
		c = defaults
	}
	if c == nil {
		return nil
	}
	to := c.To
	if to == nil && defaults != nil {
		to = defaults.To
	}
	return &IdlewatcherNotifyConfig{To: slices.Clone(to)}
}

func (c *IdlewatcherNotifyConfig) Wants() bool {
	return c != nil && (c.To == nil || len(c.To) > 0)
}
