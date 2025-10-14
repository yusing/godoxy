package logging

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/common"
	strutils "github.com/yusing/goutils/strings"

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

func NewLogger(out ...io.Writer) zerolog.Logger {
	writer := zerolog.ConsoleWriter{
		Out:        zerolog.MultiLevelWriter(out...),
		TimeFormat: timeFmt,
		FormatMessage: func(msgI interface{}) string { // pad spaces for each line
			return fmtMessage(msgI.(string))
		},
	}
	return zerolog.New(
		writer,
	).Level(level).With().Timestamp().Logger()
}

func NewLoggerWithFixedLevel(level zerolog.Level, out ...io.Writer) zerolog.Logger {
	levelStr := level.String()
	writer := zerolog.ConsoleWriter{
		Out:        zerolog.MultiLevelWriter(out...),
		TimeFormat: timeFmt,
		FormatMessage: func(msgI interface{}) string { // pad spaces for each line
			if msgI == nil {
				return ""
			}
			return fmtMessage(msgI.(string))
		},
		FormatLevel: func(_ any) string {
			return levelStr
		},
	}
	return zerolog.New(
		writer,
	).Level(level).With().Timestamp().Logger()
}

func InitLogger(out ...io.Writer) {
	logger = NewLogger(out...)
	log.SetOutput(logger)
	log.SetPrefix("")
	log.SetFlags(0)
	zerolog.TimeFieldFormat = timeFmt
	zerologlog.Logger = logger
}
