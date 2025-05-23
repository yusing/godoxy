package monitor

import (
	"net"
	"net/url"
	"time"

	"github.com/yusing/go-proxy/internal/watcher/health"
)

type (
	RawHealthMonitor struct {
		*monitor
		dialer *net.Dialer
	}
)

func NewRawHealthMonitor(url *url.URL, config *health.HealthCheckConfig) *RawHealthMonitor {
	mon := new(RawHealthMonitor)
	mon.monitor = newMonitor(url, config, mon.CheckHealth)
	mon.dialer = &net.Dialer{
		Timeout:       config.Timeout,
		FallbackDelay: -1,
	}
	return mon
}

func (mon *RawHealthMonitor) CheckHealth() (*health.HealthCheckResult, error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	url := mon.url.Load()
	start := time.Now()
	conn, err := mon.dialer.DialContext(ctx, url.Scheme, url.Host)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return &health.HealthCheckResult{
		Latency: time.Since(start),
		Healthy: true,
	}, nil
}
