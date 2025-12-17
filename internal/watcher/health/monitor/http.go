package monitor

import (
	"crypto/tls"
	"errors"
	"net/url"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/version"
)

type HTTPHealthMonitor struct {
	*monitor
	method string
}

var pinger = &fasthttp.Client{
	ReadTimeout:                   5 * time.Second,
	WriteTimeout:                  3 * time.Second,
	MaxConnDuration:               0,
	DisableHeaderNamesNormalizing: true,
	DisablePathNormalizing:        true,
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
	MaxConnsPerHost:          1,
	NoDefaultUserAgentHeader: true,
}

func NewHTTPHealthMonitor(url *url.URL, config types.HealthCheckConfig) *HTTPHealthMonitor {
	mon := new(HTTPHealthMonitor)
	mon.monitor = newMonitor(url, config, mon.CheckHealth)
	if config.UseGet {
		mon.method = fasthttp.MethodGet
	} else {
		mon.method = fasthttp.MethodHead
	}
	return mon
}

func (mon *HTTPHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(mon.url.Load().JoinPath(mon.config.Path).String())
	req.Header.SetMethod(mon.method)
	req.Header.Set("User-Agent", "GoDoxy/"+version.Get().String())
	req.Header.Set("Accept", "text/plain,text/html,*/*;q=0.8")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.SetConnectionClose()

	start := time.Now()
	respErr := pinger.DoTimeout(req, resp, mon.config.Timeout)
	lat := time.Since(start)

	if respErr != nil {
		// treat TLS error as healthy
		var tlsErr *tls.CertificateVerificationError
		if ok := errors.As(respErr, &tlsErr); !ok {
			return types.HealthCheckResult{
				Latency: lat,
				Detail:  respErr.Error(),
			}, nil
		}
	}

	if status := resp.StatusCode(); status >= 500 && status < 600 {
		return types.HealthCheckResult{
			Latency: lat,
			Detail:  fasthttp.StatusMessage(resp.StatusCode()),
		}, nil
	}

	return types.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}
