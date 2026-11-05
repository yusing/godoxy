package config

import (
	"context"
	"fmt"
	"net"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
	"github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/acl"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/entrypoint"
	"github.com/yusing/godoxy/internal/health"
	homepage "github.com/yusing/godoxy/internal/homepage/types"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	nettypes "github.com/yusing/godoxy/internal/net/types"
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
		// AddressSets are named lists of address sources (CIDR, IP, host:<name>),
		// referenced as set:<name> wherever a CIDR is accepted.
		AddressSets     map[string][]*nettypes.CIDR `json:"address_sets"`
		Providers       Providers                   `json:"providers"`
		MatchDomains    []string                    `json:"match_domains" validate:"domain_name"`
		Homepage        homepage.Config             `json:"homepage"`
		WebUI           WebUIConfig                 `json:"webui"`
		Defaults        Defaults                    `json:"defaults"`
		TimeoutShutdown int                         `json:"timeout_shutdown" validate:"gte=0"`
	}
	Defaults struct {
		HealthCheck health.HealthCheckConfig        `json:"healthcheck"`
		Idlewatcher idlewatcher.IdlewatcherDefaults `json:"idlewatcher"`
	}
	Providers struct {
		Files        []string                              `json:"include" yaml:"include,omitempty" validate:"dive,filepath"`
		Docker       map[string]types.DockerProviderConfig `json:"docker" yaml:"docker,omitempty" validate:"non_empty_docker_keys"`
		Agents       []*agent.AgentConfig                  `json:"agents" yaml:"agents,omitempty"`
		Notification []*notif.NotificationConfig           `json:"notification" yaml:"notification,omitempty"`
		Proxmox      []*proxmox.Config                     `json:"proxmox" yaml:"proxmox,omitempty"`
		MaxMind      *maxmind.Config                       `json:"maxmind" yaml:"maxmind,omitempty"`
	}
)

func Validate(data []byte) error {
	var model Config
	if err := serialization.UnmarshalValidate(data, &model, yaml.Unmarshal); err != nil {
		return err
	}
	return model.ValidateAddressSets()
}

// ValidateAddressSets checks set names and rejects sets that reference other
// sets, invalid entries, and references absent from this candidate.
func (cfg *Config) ValidateAddressSets() error {
	for name, entries := range cfg.AddressSets {
		if err := nettypes.ValidateAddressSetName(name); err != nil {
			return err
		}
		for i, entry := range entries {
			if entry == nil {
				return fmt.Errorf("address_sets.%s[%d]: expected a CIDR, IP or host:<name>", name, i)
			}
			if entry.Host() == nil && entry.Set() == nil {
				if _, _, err := net.ParseCIDR(entry.IPNet.String()); err != nil {
					return fmt.Errorf("address_sets.%s[%d]: invalid CIDR or IP: %w", name, i, err)
				}
			}
			if entry.Set() != nil {
				return fmt.Errorf("address_sets.%s[%d]: %s: address sets cannot reference other sets", name, i, entry)
			}
		}
	}
	for name, value := range map[string]any{
		"acl":                   cfg.ACL,
		"entrypoint.rules":      cfg.Entrypoint.Rules,
		"entrypoint.access_log": cfg.Entrypoint.AccessLog,
		"webui.rules":           cfg.WebUI.Rules,
		"webui.access_log":      cfg.WebUI.AccessLog,
	} {
		if err := nettypes.ValidateAddressSetReferences(value, cfg.CheckAddressSet); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if len(cfg.Entrypoint.Middlewares) != 0 {
		chain, err := middleware.BuildMiddlewareFromChainRaw("entrypoint", cfg.Entrypoint.Middlewares)
		if err != nil {
			return fmt.Errorf("entrypoint.middlewares: %w", err)
		}
		if err := chain.ValidateAddressSetReferences(cfg.CheckAddressSet); err != nil {
			return fmt.Errorf("entrypoint.middlewares: %w", err)
		}
	}
	if err := middleware.ValidateAddressSetMap(cfg.WebUI.Middlewares, cfg.CheckAddressSet); err != nil {
		return fmt.Errorf("webui.middlewares: %w", err)
	}
	return nil
}

// CheckAddressSet resolves against this candidate, never a previous runtime.
func (cfg *Config) CheckAddressSet(name string) error {
	if _, ok := cfg.AddressSets[name]; !ok {
		return fmt.Errorf("%w: set:%s", nettypes.ErrUndefinedAddressSet, name)
	}
	return nil
}

// AddressSetCheck uses the request or candidate's definitions when available.
func AddressSetCheck(ctx context.Context) nettypes.AddressSetValidator {
	if state := FromCtx(ctx); state != nil {
		return state.Value().CheckAddressSet
	}
	return nettypes.PublishedAddressSetValidator
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
