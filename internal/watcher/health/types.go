package health

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/task"
)

type (
	HealthCheckResult struct {
		Healthy bool          `json:"healthy"`
		Detail  string        `json:"detail"`
		Latency time.Duration `json:"latency"`
	}
	WithHealthInfo interface {
		Status() Status
		Uptime() time.Duration
		Latency() time.Duration
	}
	HealthMonitor interface {
		task.TaskStarter
		task.TaskFinisher
		fmt.Stringer
		json.Marshaler
		WithHealthInfo
		Name() string
	}
	HealthChecker interface {
		CheckHealth() (result *HealthCheckResult, err error)
		URL() *types.URL
		Config() *HealthCheckConfig
		UpdateURL(url *types.URL)
	}
)
