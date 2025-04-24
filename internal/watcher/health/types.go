package health

import (
	"fmt"
	"net/url"
	"time"

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
		WithHealthInfo
		Name() string
	}
	HealthChecker interface {
		CheckHealth() (result *HealthCheckResult, err error)
		URL() *url.URL
		Config() *HealthCheckConfig
		UpdateURL(url *url.URL)
	}
	HealthMonCheck interface {
		HealthMonitor
		HealthChecker
	}
)
