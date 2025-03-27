package gperr

import (
	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/logging"
)

func log(msg string, err error, level zerolog.Level, logger ...*zerolog.Logger) {
	var l *zerolog.Logger
	if len(logger) > 0 {
		l = logger[0]
	} else {
		l = logging.GetLogger()
	}
	l.WithLevel(level).Msg(msg + ": " + err.Error())
}

func LogFatal(msg string, err error, logger ...*zerolog.Logger) {
	if common.IsDebug {
		LogPanic(msg, err, logger...)
	}
	log(msg, err, zerolog.FatalLevel, logger...)
}

func LogError(msg string, err error, logger ...*zerolog.Logger) {
	log(msg, err, zerolog.ErrorLevel, logger...)
}

func LogWarn(msg string, err error, logger ...*zerolog.Logger) {
	log(msg, err, zerolog.WarnLevel, logger...)
}

func LogPanic(msg string, err error, logger ...*zerolog.Logger) {
	log(msg, err, zerolog.PanicLevel, logger...)
}

func LogDebug(msg string, err error, logger ...*zerolog.Logger) {
	log(msg, err, zerolog.DebugLevel, logger...)
}
