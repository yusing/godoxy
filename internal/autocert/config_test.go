package autocert

import (
	"fmt"
	"testing"

	"github.com/yusing/go-proxy/internal/serialization"
)

func TestEABConfigRequired(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{name: "Missing EABKid", cfg: &Config{EABHmac: "1234567890"}, wantErr: true},
		{name: "Missing EABHmac", cfg: &Config{EABKid: "1234567890"}, wantErr: true},
		{name: "Valid EAB", cfg: &Config{EABKid: "1234567890", EABHmac: "1234567890"}, wantErr: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			yaml := fmt.Appendf(nil, "eab_kid: %s\neab_hmac: %s", test.cfg.EABKid, test.cfg.EABHmac)
			cfg := Config{}
			err := serialization.UnmarshalValidateYAML(yaml, &cfg)
			if (err != nil) != test.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
