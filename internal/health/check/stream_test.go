package healthcheck

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStreamReturnsUnhealthyForInvalidURL(t *testing.T) {
	tests := []struct {
		name   string
		url    *url.URL
		detail string
	}{
		{name: "nil", url: nil, detail: "no url specified"},
		{name: "no host", url: &url.URL{Scheme: "tcp"}, detail: "no host specified"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Stream(t.Context(), tt.url, time.Hour)
			require.NoError(t, err)
			require.False(t, result.Healthy)
			require.Equal(t, tt.detail, result.Detail)
		})
	}
}
