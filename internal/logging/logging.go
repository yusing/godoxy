package logging

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/common"

	"github.com/rs/zerolog/diode"
	zerologlog "github.com/rs/zerolog/log"
)

func InitLogger(out ...io.Writer) {
	logger = NewLogger(out...)
	log.SetOutput(logger)
	log.SetPrefix("")
	log.SetFlags(0)
	zerolog.TimeFieldFormat = timeFmt
	zerologlog.Logger = logger
}

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
	nLines := strings.Count(msg, "\n")
	if nLines == 0 {
		return msg
	}

	var sb strings.Builder
	sb.Grow(len(msg) + nLines*len(prefix))

	// write first line unindented
	idx := strings.IndexByte(msg, '\n')
	sb.WriteString(msg[:idx])
	sb.WriteByte('\n')
	msg = msg[idx+1:]

	// write remaining lines indented
	for line := range strings.Lines(msg) {
		sb.WriteString(prefix)
		sb.WriteString(line)
	}
	return sb.String()
}

func diodeMultiWriter(out ...io.Writer) io.Writer {
	return diode.NewWriter(multiWriter(out...), 1024, 0, func(missed int) {
		zerologlog.Warn().Int("missed", missed).Msg("missed log messages")
	})
}

func multiWriter(out ...io.Writer) io.Writer {
	if len(out) == 0 {
		return os.Stdout
	}
	if len(out) == 1 {
		return out[0]
	}
	return io.MultiWriter(out...)
}

func NewLogger(out ...io.Writer) zerolog.Logger {
	writer := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = diodeMultiWriter(out...)
		w.TimeFormat = timeFmt
		w.FormatMessage = func(msgI any) string { // pad spaces for each line
			if msgI == nil {
				return ""
			}
			return fmtMessage(msgI.(string))
		}
	})
	return zerolog.New(writer).Level(level).With().Timestamp().Logger()
}

func NewLoggerWithFixedLevel(lvl zerolog.Level, out ...io.Writer) zerolog.Logger {
	writer := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = diodeMultiWriter(out...)
		w.TimeFormat = timeFmt
		w.FormatMessage = func(msgI any) string { // pad spaces for each line
			if msgI == nil {
				return ""
			}
			return fmtMessage(msgI.(string))
		}
	})
	return zerolog.New(writer).Level(level).With().Str("level", lvl.String()).Timestamp().Logger()
}
