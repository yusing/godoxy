package idlewatcher

import (
	"testing"

	expect "github.com/yusing/go-proxy/internal/utils/testing"
)

func TestValidateStartEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid",
			input:   "/start",
			wantErr: false,
		},
		{
			name:    "invalid",
			input:   "../foo",
			wantErr: true,
		},
		{
			name:    "single fragment",
			input:   "#",
			wantErr: true,
		},
		{
			name:    "empty",
			input:   "",
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := new(Config)
			cfg.StartEndpoint = tc.input
			err := cfg.validateStartEndpoint()
			if err == nil {
				expect.Equal(t, cfg.StartEndpoint, tc.input)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("validateStartEndpoint() error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}
