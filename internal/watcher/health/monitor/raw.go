package monitor

import (
	"errors"
	"net"
	"net/url"
	"syscall"
	"time"

	"github.com/yusing/godoxy/internal/types"
)

type (
	RawHealthMonitor struct {
		*monitor
		dialer *net.Dialer
	}
)

func NewRawHealthMonitor(url *url.URL, config *types.HealthCheckConfig) *RawHealthMonitor {
	mon := new(RawHealthMonitor)
	mon.monitor = newMonitor(url, config, mon.CheckHealth)
	mon.dialer = &net.Dialer{
		Timeout:       config.Timeout,
		FallbackDelay: -1,
	}
	return mon
}

func (mon *RawHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	url := mon.url.Load()
	start := time.Now()
	conn, err := mon.dialer.DialContext(ctx, url.Scheme, url.Host)
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
