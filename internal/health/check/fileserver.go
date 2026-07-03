package healthcheck

import (
	"os"
	"time"

	"github.com/yusing/godoxy/internal/health"
)

func FileServer(path string) (health.HealthCheckResult, error) {
	start := time.Now()
	_, err := os.Stat(path)
	lat := time.Since(start)

	if err != nil {
		if os.IsNotExist(err) {
			return health.HealthCheckResult{
				Detail: err.Error(),
			}, nil
		}
		return health.HealthCheckResult{}, err
	}

	return health.HealthCheckResult{
		Healthy: true,
		Latency: lat,
	}, nil
}
