package healthcheck

import (
	"os"
	"time"

	"github.com/yusing/godoxy/internal/types"
)

func FileServer(path string) (types.HealthCheckResult, error) {
	start := time.Now()
	_, err := os.Stat(path)
	lat := time.Since(start)

	if err != nil {
		if os.IsNotExist(err) {
			return types.HealthCheckResult{
				Detail: err.Error(),
			}, nil
		}
		return types.HealthCheckResult{}, err
	}

	return types.HealthCheckResult{
		Healthy: true,
		Latency: lat,
	}, nil
}
