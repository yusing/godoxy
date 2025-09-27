package maxmind

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	gperr "github.com/yusing/goutils/errs"
)

type (
	DatabaseType string
	Config       struct {
		AccountID  string       `json:"account_id" validate:"required"`
		LicenseKey string       `json:"license_key" validate:"required"`
		Database   DatabaseType `json:"database" validate:"omitempty,oneof=geolite geoip2"`
	}
)

const (
	MaxMindGeoLite DatabaseType = "geolite"
	MaxMindGeoIP2  DatabaseType = "geoip2"
)

func (cfg *Config) Validate() gperr.Error {
	if cfg.Database == "" {
		cfg.Database = MaxMindGeoLite
	}
	return nil
}

func (cfg *Config) Logger() *zerolog.Logger {
	l := log.With().Str("database", string(cfg.Database)).Logger()
	return &l
}
