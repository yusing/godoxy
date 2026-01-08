package healthcheck

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

var h2cClient = &http.Client{
	Transport: &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	},
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

func HTTP(url *url.URL, method, path string, timeout time.Duration) (types.HealthCheckResult, error) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(url.JoinPath(path).String())
	req.Header.SetMethod(method)
	setCommonHeaders(req.Header.Set)
	req.SetConnectionClose()

	start := time.Now()
	respErr := pinger.DoTimeout(req, resp, timeout)
	lat := time.Since(start)

	return processHealthResponse(lat, respErr, resp.StatusCode)
}

func H2C(ctx context.Context, url *url.URL, method, path string, timeout time.Duration) (types.HealthCheckResult, error) {
	u := url.JoinPath(path) // JoinPath returns a copy of the URL with the path joined
	u.Scheme = "http"

	ctx, cancel := context.WithTimeoutCause(ctx, timeout, errors.New("h2c health check timed out"))
	defer cancel()

	var req *http.Request
	var err error
	if method == fasthttp.MethodGet {
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
