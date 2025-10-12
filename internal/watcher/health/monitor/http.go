package monitor

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/version"
)

type HTTPHealthMonitor struct {
	*monitor
	method string
}

var pinger = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     10 * time.Second,
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

func (mon *HTTPHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	ctx, cancel := mon.ContextWithTimeout("ping request timed out")
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		mon.method,
		mon.url.Load().JoinPath(mon.config.Path).String(),
		nil,
	)
	if err != nil {
		return types.HealthCheckResult{}, err
	}
	req.Close = true
	req.Header.Set("User-Agent", "GoDoxy/"+version.Get().String())
	req.Header.Set("Accept", "text/plain,text/html,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	start := time.Now()
	resp, respErr := pinger.Do(req)
	if respErr == nil {
		resp.Body.Close()
	}

	lat := time.Since(start)

	switch {
	case respErr != nil:
		// treat tls error as healthy
		var tlsErr *tls.CertificateVerificationError
		if ok := errors.As(respErr, &tlsErr); !ok {
			return types.HealthCheckResult{
				Latency: lat,
				Detail:  respErr.Error(),
			}, nil
		}
	case resp.StatusCode == http.StatusServiceUnavailable:
		return types.HealthCheckResult{
			Latency: lat,
			Detail:  resp.Status,
		}, nil
	}

	return types.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}
