package healthcheck

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
