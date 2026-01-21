package accesslog

import (
	"net/http"
	"time"

	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type (
	ConfigBase struct {
		Path           string        `json:"path"`
		Stdout         bool          `json:"stdout"`
		Retention      *Retention    `json:"retention" aliases:"keep"`
		RotateInterval time.Duration `json:"rotate_interval,omitempty" swaggertype:"primitive,integer"`
	} // @name AccessLoggerConfigBase
	ACLLoggerConfig struct {
		ConfigBase
		LogAllowed bool `json:"log_allowed"`
	} // @name ACLLoggerConfig
	RequestLoggerConfig struct {
		ConfigBase
		Format  Format  `json:"format" validate:"oneof=common combined json"`
		Filters Filters `json:"filters"`
		Fields  Fields  `json:"fields"`
	} // @name RequestLoggerConfig
	Config struct {
		ConfigBase
		acl *ACLLoggerConfig
		req *RequestLoggerConfig
	}
	AnyConfig interface {
		ToConfig() *Config
		Writers() ([]File, error)
	}

	Format  string
	Filters struct {
		StatusCodes LogFilter[*StatusCodeRange] `json:"status_codes,omitzero"`
		Method      LogFilter[HTTPMethod]       `json:"method,omitzero"`
		Host        LogFilter[Host]             `json:"host,omitzero"`
		Headers     LogFilter[*HTTPHeader]      `json:"headers,omitzero"` // header exists or header == value
		CIDR        LogFilter[*CIDR]            `json:"cidr,omitzero"`
	}
	Fields struct {
		Headers FieldConfig `json:"headers,omitzero" aliases:"header"`
		Query   FieldConfig `json:"query,omitzero" aliases:"queries"`
		Cookies FieldConfig `json:"cookies,omitzero" aliases:"cookie"`
	}
)

var (
	FormatCommon   Format = "common"
	FormatCombined Format = "combined"
	FormatJSON     Format = "json"

	ReqLoggerFormats = []Format{FormatCommon, FormatCombined, FormatJSON}
)

func (cfg *ConfigBase) Validate() gperr.Error {
	if cfg.Path == "" && !cfg.Stdout {
		return gperr.New("path or stdout is required")
	}
	return nil
}

// Writers returns a list of writers for the config.
func (cfg *ConfigBase) Writers() ([]File, error) {
	writers := make([]File, 0, 2)
	if cfg.Path != "" {
		f, err := OpenFile(cfg.Path)
		if err != nil {
			return nil, err
		}
		writers = append(writers, f)
	}
	if cfg.Stdout {
		writers = append(writers, stdout)
	}
	return writers, nil
}

func (cfg *ACLLoggerConfig) ToConfig() *Config {
	return &Config{
		ConfigBase: cfg.ConfigBase,
		acl:        cfg,
	}
}

func (cfg *RequestLoggerConfig) ToConfig() *Config {
	return &Config{
		ConfigBase: cfg.ConfigBase,
		req:        cfg,
	}
}

func (cfg *Config) ShouldLogRequest(req *http.Request, res *http.Response) bool {
	if cfg.req == nil {
		return true
	}
	return cfg.req.Filters.StatusCodes.CheckKeep(req, res) &&
		cfg.req.Filters.Method.CheckKeep(req, res) &&
		cfg.req.Filters.Headers.CheckKeep(req, res) &&
		cfg.req.Filters.CIDR.CheckKeep(req, res)
}

func DefaultRequestLoggerConfig() *RequestLoggerConfig {
	return &RequestLoggerConfig{
		ConfigBase: ConfigBase{
			Retention: &Retention{Days: 30},
		},
		Format: FormatCombined,
		Fields: Fields{
			Headers: FieldConfig{
				Default: FieldModeDrop,
			},
			Query: FieldConfig{
				Default: FieldModeKeep,
			},
			Cookies: FieldConfig{
				Default: FieldModeDrop,
			},
		},
	}
}

func DefaultACLLoggerConfig() *ACLLoggerConfig {
	return &ACLLoggerConfig{
		ConfigBase: ConfigBase{
			Retention: &Retention{Days: 30},
		},
	}
}

func init() {
	serialization.RegisterDefaultValueFactory(DefaultRequestLoggerConfig)
	serialization.RegisterDefaultValueFactory(DefaultACLLoggerConfig)
}
