package logging

import (
	"github.com/rs/zerolog"
	godoxy "github.com/yusing/godoxy/internal/env"
)

// LogEnvironment reports server environment inputs, not route configuration or
// effective listener activation. It never logs unknown or sensitive values.
func LogEnvironment(logger zerolog.Logger) {
	report := godoxy.Inspect()
	for _, name := range report.Unknown {
		logger.Warn().Str("name", name).Msg("unrecognized GoDoxy environment variable; ignored as a built-in setting (custom config references are still supported)")
	}
	for _, name := range report.Deprecated {
		logger.Warn().Str("name", name).Str("replacement", "GODOXY_"+name).Msg("unprefixed environment fallback is deprecated; rename this variable (fallback remains supported)")
	}
	for _, setting := range report.Settings {
		logger.Info().Str("name", setting.Name).Str("source", setting.Source).Interface("value", setting.Value).Msg("effective server environment setting")
	}
}
