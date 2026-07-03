package healthcheck

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/yusing/godoxy/internal/health"
)

func Stream(ctx context.Context, url *url.URL, timeout time.Duration) (health.HealthCheckResult, error) {
	if result, invalid := invalidTargetURL(url); invalid {
		return result, nil
	}

	if port := url.Port(); port == "" || port == "0" {
		return health.HealthCheckResult{
			Latency: 0,
			Healthy: false,
			Detail:  "no port specified",
		}, nil
	}

	dialer := net.Dialer{
		Timeout:       timeout,
		FallbackDelay: -1,
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	conn, err := dialer.DialContext(ctx, url.Scheme, url.Host)
	lat := time.Since(start)
	if err != nil {
		if errors.Is(err, net.ErrClosed) ||
			errors.Is(err, syscall.ECONNREFUSED) ||
			errors.Is(err, syscall.ECONNRESET) ||
			errors.Is(err, syscall.ECONNABORTED) ||
			errors.Is(err, syscall.EPIPE) {
			return health.HealthCheckResult{
				Latency: lat,
				Healthy: false,
				Detail:  err.Error(),
			}, nil
		}
		return health.HealthCheckResult{}, err
	}

	defer conn.Close()
	return health.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}
