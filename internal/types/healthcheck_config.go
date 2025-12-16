package types

import (
	"context"
	"time"
)

type HealthCheckConfig struct {
	Disable  bool          `json:"disable,omitempty" aliases:"disabled"`
	UseGet   bool          `json:"use_get,omitempty"`
	Path     string        `json:"path,omitempty" validate:"omitempty,uri,startswith=/"`
	Interval time.Duration `json:"interval" validate:"omitempty,min=1s" swaggertype:"primitive,integer"`
	Timeout  time.Duration `json:"timeout" validate:"omitempty,min=1s" swaggertype:"primitive,integer"`
	Retries  int64         `json:"retries"` // <0: immediate, 0: default, >0: threshold

	BaseContext func() context.Context `json:"-"`
} //	@name	HealthCheckConfig

const (
	HealthCheckIntervalDefault        = 5 * time.Second
	HealthCheckTimeoutDefault         = 5 * time.Second
	HealthCheckDownNotifyDelayDefault = 15 * time.Second
)

func (hc *HealthCheckConfig) ApplyDefaults(defaults HealthCheckConfig) {
	if hc.Interval == 0 {
		hc.Interval = defaults.Interval
		if hc.Interval == 0 {
			hc.Interval = HealthCheckIntervalDefault
		}
	}
	if hc.Timeout == 0 {
		hc.Timeout = defaults.Timeout
		if hc.Timeout == 0 {
			hc.Timeout = HealthCheckTimeoutDefault
		}
	}
	if hc.Retries == 0 {
		hc.Retries = defaults.Retries
		if hc.Retries == 0 {
			hc.Retries = max(1, int64(HealthCheckDownNotifyDelayDefault/hc.Interval))
		}
	}
}
