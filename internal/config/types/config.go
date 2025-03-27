package types

import (
	"context"
	"regexp"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Config struct {
		AutoCert        *autocert.AutocertConfig `json:"autocert"`
		Entrypoint      Entrypoint               `json:"entrypoint"`
		Providers       Providers                `json:"providers"`
		MatchDomains    []string                 `json:"match_domains" validate:"domain_name"`
		Homepage        HomepageConfig           `json:"homepage"`
		TimeoutShutdown int                      `json:"timeout_shutdown" validate:"gte=0"`
	}
	Providers struct {
		Files        []string                   `json:"include" yaml:"include,omitempty" validate:"dive,filepath"`
		Docker       map[string]string          `json:"docker" yaml:"docker,omitempty" validate:"non_empty_docker_keys,dive,unix_addr|url"`
		Agents       []*agent.AgentConfig       `json:"agents" yaml:"agents,omitempty"`
		Notification []notif.NotificationConfig `json:"notification" yaml:"notification,omitempty"`
	}
	Entrypoint struct {
		Middlewares []map[string]any  `json:"middlewares"`
		AccessLog   *accesslog.Config `json:"access_log" validate:"omitempty"`
	}

	ConfigInstance interface {
		Value() *Config
		Reload() gperr.Error
		Statistics() map[string]any
		RouteProviderList() []string
		Context() context.Context
		GetAgent(agentAddrOrDockerHost string) (*agent.AgentConfig, bool)
		VerifyNewAgent(host string, ca agent.PEMPair, client agent.PEMPair) (int, gperr.Error)
		ListAgents() []*agent.AgentConfig
		AutoCertProvider() *autocert.Provider
	}
)

var (
	instance   ConfigInstance
	instanceMu sync.RWMutex
)

func DefaultConfig() *Config {
	return &Config{
		TimeoutShutdown: 3,
		Homepage: HomepageConfig{
			UseDefaultCategories: true,
		},
	}
}

func GetInstance() ConfigInstance {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instance
}

func SetInstance(cfg ConfigInstance) {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	instance = cfg
}

func HasInstance() bool {
	instanceMu.RLock()
	defer instanceMu.RUnlock()
	return instance != nil
}

func Validate(data []byte) gperr.Error {
	var model Config
	return utils.DeserializeYAML(data, &model)
}

var matchDomainsRegex = regexp.MustCompile(`^[^\.]?([\w\d\-_]\.?)+[^\.]?$`)

func init() {
	utils.RegisterDefaultValueFactory(DefaultConfig)
	utils.MustRegisterValidation("domain_name", func(fl validator.FieldLevel) bool {
		domains := fl.Field().Interface().([]string)
		for _, domain := range domains {
			if !matchDomainsRegex.MatchString(domain) {
				return false
			}
		}
		return true
	})
	utils.MustRegisterValidation("non_empty_docker_keys", func(fl validator.FieldLevel) bool {
		m := fl.Field().Interface().(map[string]string)
		for k := range m {
			if k == "" {
				return false
			}
		}
		return true
	})
}
