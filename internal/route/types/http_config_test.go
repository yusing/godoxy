package route_test

import (
	"testing"
	"time"

	. "github.com/yusing/go-proxy/internal/route"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/utils"
	. "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestHTTPConfigDeserialize(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected route.HTTPConfig
	}{
		{
			name: "no_tls_verify",
			input: map[string]any{
				"no_tls_verify": "true",
			},
			expected: route.HTTPConfig{
				NoTLSVerify: true,
			},
		},
		{
			name: "response_header_timeout",
			input: map[string]any{
				"response_header_timeout": "1s",
			},
			expected: route.HTTPConfig{
				ResponseHeaderTimeout: 1 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Route{}
			tt.input["host"] = "internal"
			err := utils.MapUnmarshalValidate(tt.input, &cfg)
			if err != nil {
				ExpectNoError(t, err)
			}
			ExpectEqual(t, cfg.HTTPConfig, tt.expected)
		})
	}
}
