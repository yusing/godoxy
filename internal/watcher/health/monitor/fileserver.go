package monitor

import (
	"os"
	"time"

	"github.com/yusing/go-proxy/internal/watcher/health"
)

type FileServerHealthMonitor struct {
	*monitor
	path string
}

func NewFileServerHealthMonitor(config *health.HealthCheckConfig, path string) *FileServerHealthMonitor {
	mon := &FileServerHealthMonitor{path: path}
	mon.monitor = newMonitor(nil, config, mon.CheckHealth)
	return mon
}

func (s *FileServerHealthMonitor) CheckHealth() (*health.HealthCheckResult, error) {
	start := time.Now()
	_, err := os.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &health.HealthCheckResult{
				Detail: err.Error(),
			}, nil
		}
		return nil, err
	}
	return &health.HealthCheckResult{
		Healthy: true,
		Latency: time.Since(start),
	}, nil
}
