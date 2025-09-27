package accesslog

import (
	"time"

	"github.com/yusing/godoxy/internal/serialization"
	gperr "github.com/yusing/goutils/errs"
)

type (
	ConfigBase struct {
		B              int           `json:"buffer_size"` // Deprecated: buffer size is adjusted dynamically
		Path           string        `json:"path"`
		Stdout         bool          `json:"stdout"`
		Retention      *Retention    `json:"retention" aliases:"keep"`
		RotateInterval time.Duration `json:"rotate_interval,omitempty" swaggertype:"primitive,integer"`
	}
	ACLLoggerConfig struct {
		ConfigBase
		LogAllowed bool `json:"log_allowed"`
	}
	RequestLoggerConfig struct {
		ConfigBase
		Format  Format  `json:"format" validate:"oneof=common combined json"`
		Filters Filters `json:"filters"`
		Fields  Fields  `json:"fields"`
	} // @name RequestLoggerConfig
	Config struct {
		*ConfigBase
		acl *ACLLoggerConfig
		req *RequestLoggerConfig
	}
	AnyConfig interface {
		ToConfig() *Config
		IO() (WriterWithName, error)
	}

	Format  string
	Filters struct {
		StatusCodes LogFilter[*StatusCodeRange] `json:"status_codes"`
		Method      LogFilter[HTTPMethod]       `json:"method"`
		Host        LogFilter[Host]             `json:"host"`
		Headers     LogFilter[*HTTPHeader]      `json:"headers"` // header exists or header == value
		CIDR        LogFilter[*CIDR]            `json:"cidr"`
	}
	Fields struct {
		Headers FieldConfig `json:"headers" aliases:"header"`
		Query   FieldConfig `json:"query" aliases:"queries"`
		Cookies FieldConfig `json:"cookies" aliases:"cookie"`
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

func (cfg *ConfigBase) IO() (WriterWithName, error) {
	ios := make([]WriterWithName, 0, 2)
	if cfg.Stdout {
		ios = append(ios, stdoutIO)
	}
	if cfg.Path != "" {
		io, err := newFileIO(cfg.Path)
		if err != nil {
			return nil, err
		}
		ios = append(ios, io)
	}
	if len(ios) == 0 {
		return nil, nil
	}
	return NewMultiWriter(ios...), nil
}

func (cfg *ACLLoggerConfig) ToConfig() *Config {
	return &Config{
		ConfigBase: &cfg.ConfigBase,
		acl:        cfg,
	}
}

func (cfg *RequestLoggerConfig) ToConfig() *Config {
	return &Config{
		ConfigBase: &cfg.ConfigBase,
		req:        cfg,
	}
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
