package gperr

import (
	"os"

	"github.com/rs/zerolog"
	zerologlog "github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/common"
)

func log(msg string, err error, level zerolog.Level, logger ...*zerolog.Logger) {
	var l *zerolog.Logger
	if len(logger) > 0 {
		l = logger[0]
	} else {
		l = &zerologlog.Logger
	}
	l.WithLevel(level).Msg(New(highlightANSI(msg)).With(err).Error())
	switch level {
	case zerolog.FatalLevel:
		os.Exit(1)
	case zerolog.PanicLevel:
		panic(err)
	}
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
