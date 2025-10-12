package entrypoint

import (
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/route/rules"
)

type Config struct {
	SupportProxyProtocol bool `json:"support_proxy_protocol"`
	Rules                struct {
		NotFound rules.Rules `json:"not_found"`
	} `json:"rules"`
	Middlewares []map[string]any               `json:"middlewares"`
	AccessLog   *accesslog.RequestLoggerConfig `json:"access_log" validate:"omitempty"`
}
