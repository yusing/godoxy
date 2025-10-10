package config

import (
	"regexp"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/autocert"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	homepage "github.com/yusing/godoxy/internal/homepage/types"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type (
	Config struct {
		ACL             *acl.Config       `json:"acl"`
		AutoCert        *autocert.Config  `json:"autocert"`
		Entrypoint      entrypoint.Config `json:"entrypoint"`
		Providers       Providers         `json:"providers"`
		MatchDomains    []string          `json:"match_domains" validate:"domain_name"`
		Homepage        homepage.Config   `json:"homepage"`
		TimeoutShutdown int               `json:"timeout_shutdown" validate:"gte=0"`
	}
	Providers struct {
		Files        []string                    `json:"include" yaml:"include,omitempty" validate:"dive,filepath"`
		Docker       map[string]string           `json:"docker" yaml:"docker,omitempty" validate:"non_empty_docker_keys,dive,unix_addr|url"`
		Agents       []*agent.AgentConfig        `json:"agents" yaml:"agents,omitempty"`
		Notification []*notif.NotificationConfig `json:"notification" yaml:"notification,omitempty"`
		Proxmox      []proxmox.Config            `json:"proxmox" yaml:"proxmox,omitempty"`
		MaxMind      *maxmind.Config             `json:"maxmind" yaml:"maxmind,omitempty"`
	}
)

// nil-safe
var ActiveConfig atomic.Pointer[Config]

func init() {
	ActiveConfig.Store(DefaultConfig())
}

func Validate(data []byte) gperr.Error {
	var model Config
	return serialization.UnmarshalValidateYAML(data, &model)
}

func DefaultConfig() *Config {
	return &Config{
		TimeoutShutdown: 3,
		Homepage: homepage.Config{
			UseDefaultCategories: true,
		},
	}
}

var matchDomainsRegex = regexp.MustCompile(`^[^\.]?([\w\d\-_]\.?)+[^\.]?$`)

func init() {
	serialization.RegisterDefaultValueFactory(DefaultConfig)
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
		m := fl.Field().Interface().(map[string]string)
		for k := range m {
			if k == "" {
				return false
			}
		}
		return true
	})
}
