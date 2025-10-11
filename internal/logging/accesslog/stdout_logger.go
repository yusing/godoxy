package accesslog

import (
	"github.com/rs/zerolog/log"
)

type StdoutLogger struct{}

var stdoutIO StdoutLogger

func (l StdoutLogger) Write(p []byte) (int, error) {
	return log.Logger.Write(p)
}

func (l StdoutLogger) Name() string {
	return "stdout"
}
