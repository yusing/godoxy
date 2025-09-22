package monitor

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/godoxy/pkg"
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

func NewHTTPHealthMonitor(url *url.URL, config *types.HealthCheckConfig) *HTTPHealthMonitor {
	mon := new(HTTPHealthMonitor)
	mon.monitor = newMonitor(url, config, mon.CheckHealth)
	if config.UseGet {
		mon.method = http.MethodGet
	} else {
		mon.method = http.MethodHead
	}
	return mon
}

func (mon *HTTPHealthMonitor) CheckHealth() (*types.HealthCheckResult, error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		mon.method,
		mon.url.Load().JoinPath(mon.config.Path).String(),
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Close = true
	req.Header.Set("Connection", "close")
	req.Header.Set("User-Agent", "GoDoxy/"+pkg.GetVersion().String())

	start := time.Now()
	resp, respErr := pinger.Do(req)
	if respErr == nil {
		defer resp.Body.Close()
	}

	lat := time.Since(start)

	switch {
	case respErr != nil:
		// treat tls error as healthy
		var tlsErr *tls.CertificateVerificationError
		if ok := errors.As(respErr, &tlsErr); !ok {
			return &types.HealthCheckResult{
				Latency: lat,
				Detail:  respErr.Error(),
			}, nil
		}
	case resp.StatusCode == http.StatusServiceUnavailable:
		return &types.HealthCheckResult{
			Latency: lat,
			Detail:  resp.Status,
		}, nil
	}

	return &types.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}
