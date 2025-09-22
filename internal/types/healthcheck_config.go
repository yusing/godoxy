package types

import (
	"context"
	"time"

	"github.com/yusing/godoxy/internal/common"
)

type HealthCheckConfig struct {
	Disable  bool          `json:"disable,omitempty" aliases:"disabled"`
	Path     string        `json:"path,omitempty" validate:"omitempty,uri,startswith=/"`
	UseGet   bool          `json:"use_get,omitempty"`
	Interval time.Duration `json:"interval" validate:"omitempty,min=1s" swaggertype:"primitive,integer"`
	Timeout  time.Duration `json:"timeout" validate:"omitempty,min=1s" swaggertype:"primitive,integer"`
	Retries  int64         `json:"retries"` // <0: immediate, >=0: threshold

	BaseContext func() context.Context `json:"-"`
} //	@name	HealthCheckConfig

func DefaultHealthConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Interval: common.HealthCheckIntervalDefault,
		Timeout:  common.HealthCheckTimeoutDefault,
		Retries:  int64(common.HealthCheckDownNotifyDelayDefault / common.HealthCheckIntervalDefault),
	}
}
