package accesslog

import (
	"io"
	"os"
)

type StdoutLogger struct {
	io.Writer
}

var stdoutIO = &StdoutLogger{os.Stdout}

func (l *StdoutLogger) Name() string {
	return "stdout"
}
