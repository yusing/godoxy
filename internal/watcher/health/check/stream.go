package healthcheck

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/yusing/godoxy/internal/types"
)

func Stream(ctx context.Context, url *url.URL, timeout time.Duration) (types.HealthCheckResult, error) {
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
			return types.HealthCheckResult{
				Latency: lat,
				Healthy: false,
				Detail:  err.Error(),
			}, nil
		}
		return types.HealthCheckResult{}, err
	}

	defer conn.Close()
	return types.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}
