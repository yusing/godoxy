//nolint:zerologlint
package logging

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/yusing/go-proxy/internal/common"
	"github.com/yusing/go-proxy/internal/utils/strutils"

	zerologlog "github.com/rs/zerolog/log"
)

var (
	logger  zerolog.Logger
	timeFmt string
	level   zerolog.Level
	prefix  string
)

func init() {
	switch {
	case common.IsTrace:
		timeFmt = "04:05"
		level = zerolog.TraceLevel
	case common.IsDebug:
		timeFmt = "01-02 15:04"
		level = zerolog.DebugLevel
	default:
		timeFmt = "01-02 15:04"
		level = zerolog.InfoLevel
	}
	prefixLength := len(timeFmt) + 5 // level takes 3 + 2 spaces
	prefix = strings.Repeat(" ", prefixLength)
	InitLogger(os.Stdout)
}

func fmtMessage(msg string) string {
	lines := strutils.SplitRune(msg, '\n')
	if len(lines) == 1 {
		return msg
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strutils.JoinRune(lines, '\n')
}

func InitLogger(out ...io.Writer) {
	writer := zerolog.ConsoleWriter{
		Out:        zerolog.MultiLevelWriter(out...),
		TimeFormat: timeFmt,
		FormatMessage: func(msgI interface{}) string { // pad spaces for each line
			return fmtMessage(msgI.(string))
		},
	}
	logger = zerolog.New(
		writer,
	).Level(level).With().Timestamp().Logger()
	log.SetOutput(writer)
	log.SetPrefix("")
	log.SetFlags(0)
	zerolog.TimeFieldFormat = timeFmt
	zerologlog.Logger = logger
}
