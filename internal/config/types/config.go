package config

import (
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/entrypoint"
	homepage "github.com/yusing/godoxy/internal/homepage/types"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/serialization"
	"github.com/yusing/godoxy/internal/types"
)

type (
	Config struct {
		ACL                 *acl.Config                         `json:"acl"`
		AutoCert            *autocert.Config                    `json:"autocert"`
		Entrypoint          entrypoint.Config                   `json:"entrypoint"`
		InboundMTLSProfiles map[string]types.InboundMTLSProfile `json:"inbound_mtls_profiles"`
		Providers           Providers                           `json:"providers"`
		MatchDomains        []string                            `json:"match_domains" validate:"domain_name"`
		Homepage            homepage.Config                     `json:"homepage"`
		WebUI               WebUIConfig                         `json:"webui"`
		Defaults            Defaults                            `json:"defaults"`
		TimeoutShutdown     int                                 `json:"timeout_shutdown" validate:"gte=0"`
	}
	Defaults struct {
		HealthCheck types.HealthCheckConfig `json:"healthcheck"`
	}
	Providers struct {
		Files        []string                              `json:"include" yaml:"include,omitempty" validate:"dive,filepath"`
		Docker       map[string]types.DockerProviderConfig `json:"docker" yaml:"docker,omitempty" validate:"non_empty_docker_keys"`
		Agents       []*agent.AgentConfig                  `json:"agents" yaml:"agents,omitempty"`
		Notification []*notif.NotificationConfig           `json:"notification" yaml:"notification,omitempty"`
		Proxmox      []*proxmox.Config                     `json:"proxmox" yaml:"proxmox,omitempty"`
		MaxMind      *maxmind.Config                       `json:"maxmind" yaml:"maxmind,omitempty"`
	}
	WebUIConfig struct {
		InboundMTLSProfile string                         `json:"inbound_mtls_profile,omitempty"`
		Middlewares        map[string]types.LabelMap      `json:"middlewares,omitempty" extensions:"x-nullable"`
		AccessLog          *accesslog.RequestLoggerConfig `json:"access_log,omitempty" extensions:"x-nullable"`

		Aliases []string `json:"aliases"`
	}
)

func Validate(data []byte) error {
	var model Config
	return serialization.UnmarshalValidate(data, &model, yaml.Unmarshal)
}

func DefaultConfig() Config {
	return Config{
		TimeoutShutdown: 3,
		Homepage: homepage.Config{
			UseDefaultCategories: true,
		},
	}
}

var matchDomainsRegex = regexp.MustCompile(`^[^\.]?([\w\d\-_]\.?)+[^\.]?$`)

func init() {
	serialization.MustRegisterValidation("domain_name", func(fl validator.FieldLevel) bool {
		domains := fl.Field().Interface().([]string)
		for _, domain := range domains {
			if !matchDomainsRegex.MatchString(domain) {
				return false
			}
		}
		return true
	})
	serialization.MustRegisterValidation("non_empty_docker_keys", func(fl validator.FieldLevel) bool {
		m := fl.Field().Interface().(map[string]types.DockerProviderConfig)
		for k := range m {
			if k == "" {
				return false
			}
		}
		return true
	})
}
