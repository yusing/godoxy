package monitor

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"

	"github.com/yusing/go-proxy/internal/net/types"
	"github.com/yusing/go-proxy/internal/watcher/health"
	"github.com/yusing/go-proxy/pkg"
)

type HTTPHealthMonitor struct {
	*monitor
	method string
}

var pinger = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives: true,
		ForceAttemptHTTP2: false,
	},
	CheckRedirect: func(r *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func NewHTTPHealthMonitor(url *types.URL, config *health.HealthCheckConfig) *HTTPHealthMonitor {
	mon := new(HTTPHealthMonitor)
	mon.monitor = newMonitor(url, config, mon.CheckHealth)
	if config.UseGet {
		mon.method = http.MethodGet
	} else {
		mon.method = http.MethodHead
	}
	return mon
}

func (mon *HTTPHealthMonitor) CheckHealth() (result *health.HealthCheckResult, err error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	req, reqErr := http.NewRequestWithContext(
		ctx,
		mon.method,
		mon.url.Load().JoinPath(mon.config.Path).String(),
		nil,
	)
	if reqErr != nil {
		err = reqErr
		return
	}
	req.Close = true
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "GoDoxy/"+pkg.GetVersion())

	start := time.Now()
	resp, respErr := pinger.Do(req)
	if respErr == nil {
		defer resp.Body.Close()
	}

	lat := time.Since(start)
	result = &health.HealthCheckResult{}

	switch {
	case respErr != nil:
		// treat tls error as healthy
		var tlsErr *tls.CertificateVerificationError
		if ok := errors.As(respErr, &tlsErr); !ok {
			result.Detail = respErr.Error()
			return
		}
	case resp.StatusCode == http.StatusServiceUnavailable:
		result.Detail = resp.Status
		return
	}

	result.Latency = lat
	result.Healthy = true
	return
}
