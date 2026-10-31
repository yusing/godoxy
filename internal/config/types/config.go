package config

import (
	"fmt"
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

// StageAddressSetNames reads only the address_sets keys from a config
// document and stages them, so set:<name> references parsed while the rest of
// the document loads do not warn about sets that are about to be published.
func StageAddressSetNames(data []byte) {
	var doc struct {
		AddressSets map[string]any `yaml:"address_sets"`
	}
	if yaml.Unmarshal(data, &doc) != nil {
		return
	}
	names := make([]string, 0, len(doc.AddressSets))
	for name := range doc.AddressSets {
		names = append(names, name)
	}
	nettypes.StageAddressSetNames(names)
}

// ValidateAddressSets checks set names and rejects sets that reference other
// sets.
func (cfg *Config) ValidateAddressSets() error {
	for name, entries := range cfg.AddressSets {
		if err := nettypes.ValidateAddressSetName(name); err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Set() != nil {
				return fmt.Errorf("address_sets.%s: %s: address sets cannot reference other sets", name, entry)
			}
		}
	}
	return nil
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
