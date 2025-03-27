package types

import (
	"context"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/yusing/go-proxy/internal/autocert"
	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/notif"
	"github.com/yusing/go-proxy/internal/utils"

	E "github.com/yusing/go-proxy/internal/error"
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
		Files        []string                   `json:"include" validate:"dive,filepath"`
		Docker       map[string]string          `json:"docker" validate:"dive,unix_addr|url"`
		Notification []notif.NotificationConfig `json:"notification"`
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
	}
)

func DefaultConfig() *Config {
	return &Config{
		TimeoutShutdown: 3,
		Homepage: HomepageConfig{
			UseDefaultCategories: true,
		},
	}
}

func Validate(data []byte) E.Error {
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
}
