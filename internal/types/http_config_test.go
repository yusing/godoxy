package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/serialization"
	. "github.com/yusing/godoxy/internal/types"
)

func TestHTTPConfigDeserialize(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected HTTPConfig
	}{
		{
			name: "no_tls_verify",
			input: map[string]any{
				"no_tls_verify": "true",
			},
			expected: HTTPConfig{
				NoTLSVerify: true,
			},
		},
		{
			name: "response_header_timeout",
			input: map[string]any{
				"response_header_timeout": "1s",
			},
			expected: HTTPConfig{
				ResponseHeaderTimeout: 1 * time.Second,
			},
		},
		{
			name: "max_conns_per_host",
			input: map[string]any{
				"max_conns_per_host": "256",
			},
			expected: HTTPConfig{
				MaxConnsPerHost: 256,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := route.Route{}
			tt.input["host"] = "internal"
			err := serialization.MapUnmarshalValidate(tt.input, &cfg)
			if err != nil {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, cfg.HTTPConfig)
		})
	}
}
