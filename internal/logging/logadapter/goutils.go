// Package logadapter connects goutils diagnostics to GoDoxy's zerolog logger.
package logadapter

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/logging"
)

// Register routes goutils diagnostics through the current global zerolog logger.
// Call it after configuring that logger.
func Register() {
	logging.SetLogger(adapter{})
}

type adapter struct{}

func (adapter) Log(level logging.Level, message string, fields ...logging.Field) {
	var severity zerolog.Level
	switch level {
	case logging.Debug:
		severity = zerolog.DebugLevel
	case logging.Info:
		severity = zerolog.InfoLevel
	case logging.Warn:
		severity = zerolog.WarnLevel
	case logging.Error:
		severity = zerolog.ErrorLevel
	default:
		return
	}
	event := log.WithLevel(severity)
	if !event.Enabled() {
		return
	}
	for _, field := range fields {
		if err, ok := field.Value.(error); ok {
			event = event.AnErr(field.Key, err)
		} else {
			event = event.Interface(field.Key, field.Value)
		}
	}
	event.Msg(message)
}
