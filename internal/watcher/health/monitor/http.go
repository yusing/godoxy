package monitor

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/version"
	"golang.org/x/net/http2"
)

type HTTPHealthMonitor struct {
	*monitor
	method string
}

var pinger = &fasthttp.Client{
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

var userAgent = "GoDoxy/" + version.Get().String()

func setCommonHeaders(setHeader func(key, value string)) {
	setHeader("User-Agent", userAgent)
	setHeader("Accept", "text/plain,text/html,*/*;q=0.8")
	setHeader("Accept-Encoding", "identity")
	setHeader("Cache-Control", "no-cache")
	setHeader("Pragma", "no-cache")
}

func processHealthResponse(lat time.Duration, err error, getStatusCode func() int) (types.HealthCheckResult, error) {
	if err != nil {
		var tlsErr *tls.CertificateVerificationError
		if ok := errors.As(err, &tlsErr); !ok {
			return types.HealthCheckResult{
				Latency: lat,
				Detail:  err.Error(),
			}, nil
		}
	}

	statusCode := getStatusCode()
	if statusCode >= 500 && statusCode < 600 {
		return types.HealthCheckResult{
			Latency: lat,
			Detail:  http.StatusText(statusCode),
		}, nil
	}

	return types.HealthCheckResult{
		Latency: lat,
		Healthy: true,
	}, nil
}

var h2cClient = &http.Client{
	Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	},
}

func (mon *HTTPHealthMonitor) CheckHealth() (types.HealthCheckResult, error) {
	if mon.url.Load().Scheme == "h2c" {
		return mon.CheckHealthH2C()
	}
	return mon.CheckHealthHTTP()
}

func (mon *HTTPHealthMonitor) CheckHealthHTTP() (types.HealthCheckResult, error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(mon.url.Load().JoinPath(mon.config.Path).String())
	req.Header.SetMethod(mon.method)
	setCommonHeaders(req.Header.Set)
	req.SetConnectionClose()

	start := time.Now()
	respErr := pinger.DoTimeout(req, resp, mon.config.Timeout)
	lat := time.Since(start)

	return processHealthResponse(lat, respErr, resp.StatusCode)
}

func (mon *HTTPHealthMonitor) CheckHealthH2C() (types.HealthCheckResult, error) {
	u := mon.url.Load()
	u = u.JoinPath(mon.config.Path) // JoinPath returns a copy of the URL with the path joined
	u.Scheme = "http"

	ctx, cancel := mon.ContextWithTimeout("h2c health check timed out")
	defer cancel()

	var req *http.Request
	var err error
	if mon.method == fasthttp.MethodGet {
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	}
	if err != nil {
		return types.HealthCheckResult{
			Detail: err.Error(),
		}, nil
	}

	setCommonHeaders(req.Header.Set)

	start := time.Now()
	resp, err := h2cClient.Do(req)
	lat := time.Since(start)

	if resp != nil {
		defer resp.Body.Close()
	}

	return processHealthResponse(lat, err, func() int { return resp.StatusCode })
}
