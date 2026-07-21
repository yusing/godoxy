package entrypoint

import (
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/goutils/server"
)

// Config defines the entrypoint configuration for proxy handling,
// including proxy protocol support, routing rules, middlewares, and access logging.
type Config struct {
	// SupportProxyProtocol is the deprecated legacy setting for optional
	// PROXY-header detection. Configure ProxyProtocol instead.
	SupportProxyProtocol bool `json:"support_proxy_protocol"`
	// ProxyProtocol configures durable source-authenticated PROXY handling.
	ProxyProtocol      *server.ProxyProtocolConfig `json:"proxy_protocol,omitempty"`
	InboundMTLSProfile string                      `json:"inbound_mtls_profile,omitempty"`
	Rules              struct {
		NotFound rules.Rules `json:"not_found"`
	} `json:"rules"`
	Middlewares []map[string]any               `json:"middlewares"`
	AccessLog   *accesslog.RequestLoggerConfig `json:"access_log"`
}

func (cfg *Config) Validate() error {
	if cfg.ProxyProtocol == nil {
		return nil
	}
	return cfg.ProxyProtocol.Validate()
}
