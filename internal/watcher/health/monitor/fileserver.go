package monitor

import (
	"os"
	"time"

	"github.com/yusing/go-proxy/internal/types"
)

type FileServerHealthMonitor struct {
	*monitor
	path string
}

func NewFileServerHealthMonitor(config *types.HealthCheckConfig, path string) *FileServerHealthMonitor {
	mon := &FileServerHealthMonitor{path: path}
	mon.monitor = newMonitor(nil, config, mon.CheckHealth)
	return mon
}

func (s *FileServerHealthMonitor) CheckHealth() (*types.HealthCheckResult, error) {
	start := time.Now()
	_, err := os.Stat(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.HealthCheckResult{
				Detail: err.Error(),
			}, nil
		}
		return nil, err
	}
	return &types.HealthCheckResult{
		Healthy: true,
		Latency: time.Since(start),
	}, nil
}
