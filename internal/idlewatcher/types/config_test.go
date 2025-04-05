package idlewatcher

import (
	"testing"

	. "github.com/yusing/go-proxy/internal/utils/testing"
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
			s, err := validateStartEndpoint(tc.input)
			if err == nil {
				ExpectEqual(t, s, tc.input)
			}
			if (err != nil) != tc.wantErr {
				t.Errorf("validateStartEndpoint() error = %v, wantErr %t", err, tc.wantErr)
			}
		})
	}
}
