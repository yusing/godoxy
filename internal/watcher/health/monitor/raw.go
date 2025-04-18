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

func (mon *RawHealthMonitor) CheckHealth() (result *health.HealthCheckResult, err error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	url := mon.url.Load()
	start := time.Now()
	conn, dialErr := mon.dialer.DialContext(ctx, url.Scheme, url.Host)
	result = new(health.HealthCheckResult)
	if dialErr != nil {
		result.Detail = dialErr.Error()
		return
	}
	defer conn.Close()
	result.Latency = time.Since(start)
	result.Healthy = true
	return
}
