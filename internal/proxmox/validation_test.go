package proxmox

import (
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/serialization"
)

func TestValidateCommandArgs(t *testing.T) {
	tests := []struct {
		name    string
		yamlCfg string
		wantErr bool
	}{
		{
			name:    "valid_services",
			yamlCfg: `services: ["foo", "bar"]`,
			wantErr: false,
		},
		{
			name:    "invalid_services",
			yamlCfg: `services: ["foo", "bar & baz"]`,
			wantErr: true,
		},
		{
			name:    "invalid_services_with_$(",
			yamlCfg: `services: ["foo", "bar & $(echo 'hello')"]`,
			wantErr: true,
		},
		{
			name:    "valid_files",
			yamlCfg: `files: ["foo", "bar"]`,
			wantErr: false,
		},
		{
			name:    "invalid_files",
			yamlCfg: `files: ["foo", "bar & baz"]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg NodeConfig
			err := serialization.UnmarshalValidate([]byte(tt.yamlCfg), &cfg, yaml.Unmarshal)
			if tt.wantErr {
				require.Error(t, err)
				require.ErrorContains(t, err, "input contains invalid characters")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
