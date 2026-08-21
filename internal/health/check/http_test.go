package healthcheck

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestHTTPReturnsUnhealthyForInvalidURL(t *testing.T) {
	tests := []struct {
		name   string
		url    *url.URL
		detail string
	}{
		{name: "nil", url: nil, detail: "no url specified"},
		{name: "no host", url: &url.URL{Scheme: "http"}, detail: "no host specified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := HTTP(tt.url, http.MethodHead, "/", time.Hour)
			require.NoError(t, err)
			require.False(t, result.Healthy)
			require.Equal(t, tt.detail, result.Detail)
		})
	}
}

func TestH2CReturnsUnhealthyForInvalidURL(t *testing.T) {
	tests := []struct {
		name   string
		url    *url.URL
		detail string
	}{
		{name: "nil", url: nil, detail: "no url specified"},
		{name: "no host", url: &url.URL{Scheme: "h2c"}, detail: "no host specified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := H2C(t.Context(), tt.url, http.MethodHead, "/", time.Hour)
			require.NoError(t, err)
			require.False(t, result.Healthy)
			require.Equal(t, tt.detail, result.Detail)
		})
	}
}

func TestProcessHealthResponseCompactsRedundantDialError(t *testing.T) {
	dialer := fasthttp.TCPDialer{DisableDNSResolution: true}
	_, err := dialer.DialTimeout("127.0.0.1:0", time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error when dialing 127.0.0.1:0:")

	innerErr := errors.Unwrap(err)
	require.Error(t, innerErr)

	result := processHealthResponse(time.Second, err, func() int { return 0 })
	require.Equal(t, innerErr.Error(), result.Detail)
	require.Equal(t, time.Second, result.Latency)
}

func TestProcessHealthResponsePreservesNonDialError(t *testing.T) {
	err := errors.New("request failed")

	result := processHealthResponse(time.Second, err, func() int { return 0 })
	require.Equal(t, err.Error(), result.Detail)
	require.Equal(t, time.Second, result.Latency)
}
