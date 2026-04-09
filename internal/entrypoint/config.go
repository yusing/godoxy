package entrypoint

import (
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
)

// Config defines the entrypoint configuration for proxy handling,
// including proxy protocol support, routing rules, middlewares, and access logging.
type Config struct {
	SupportProxyProtocol bool   `json:"support_proxy_protocol"`
	InboundMTLSProfile   string `json:"inbound_mtls_profile,omitempty"`
	Rules                struct {
		NotFound rules.Rules `json:"not_found"`
	} `json:"rules"`
	Middlewares []map[string]any               `json:"middlewares"`
	AccessLog   *accesslog.RequestLoggerConfig `json:"access_log"`
}
