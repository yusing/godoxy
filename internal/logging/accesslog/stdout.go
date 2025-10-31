package accesslog

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/logging"
)

type Stdout struct {
	logger zerolog.Logger
}

func NewStdout() Writer {
	return &Stdout{logger: logging.NewLoggerWithFixedLevel(zerolog.InfoLevel, os.Stdout)}
}

func (s Stdout) Name() string {
	return "stdout"
}

func (s Stdout) ShouldBeBuffered() bool {
	return false
}

func (s Stdout) Write(p []byte) (n int, err error) {
	return s.logger.Write(p)
}

func (s Stdout) Close() error {
	return nil
}
