package types

import "errors"

type InboundMTLSProfile struct {
	UseSystemCAs bool     `json:"use_system_cas,omitempty" yaml:"use_system_cas,omitempty"`
	CAFiles      []string `json:"ca_files,omitempty" yaml:"ca_files,omitempty" validate:"omitempty,dive,filepath"`
}

func (cfg InboundMTLSProfile) Validate() error {
	if !cfg.UseSystemCAs && len(cfg.CAFiles) == 0 {
		return errors.New("at least one trust source is required for inbound mTLS profile")
	}
	return nil
}
