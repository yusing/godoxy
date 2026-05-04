package config

import (
	"errors"
	"strings"

	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/types"
)

type WebUIConfig struct {
	InboundMTLSProfile string                         `json:"inbound_mtls_profile,omitempty"`
	Middlewares        map[string]types.LabelMap      `json:"middlewares,omitempty" extensions:"x-nullable"`
	AccessLog          *accesslog.RequestLoggerConfig `json:"access_log,omitempty" extensions:"x-nullable"`
	Rules              rules.Rules                    `json:"rules,omitempty" extensions:"x-nullable"`

	Aliases []string `json:"aliases"`
}

func (cfg *WebUIConfig) Validate() error {
	for _, alias := range cfg.Aliases {
		if strings.TrimSpace(alias) == "" {
			return errors.New("empty alias is not allowed")
		}
	}
	return nil
}
