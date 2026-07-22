package logging

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"github.com/yusing/godoxy/internal/common"

	zerologlog "github.com/rs/zerolog/log"
)

func InitLogger(out ...io.Writer) {
	destinations := processDestinations(out...)
	outputMu.Lock()
	defaultOutputs = destinations
	outputMu.Unlock()

	logger = newLogger(processWriter{})
	log.SetOutput(logger)
	log.SetPrefix("")
	log.SetFlags(0)
	zerolog.TimeFieldFormat = timeFmt
	zerologlog.Logger = logger
}

var (
	logger         zerolog.Logger
	timeFmt        string
	level          zerolog.Level
	prefix         string
	outputMu       sync.Mutex
	defaultOutputs = []io.Writer{os.Stdout}
)

type processWriter struct{}

func (processWriter) Write(p []byte) (int, error) {
	return writeOutput(p)
}

type lockedWriter struct {
	mu  sync.Mutex
	out io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.out.Write(p)
}

// Buffer owns log output produced by one operation until it is either flushed
// to the process log or discarded. Its zero value is ready to use.
type Buffer struct {
	mu           sync.Mutex
	buf          bytes.Buffer
	destinations []bufferDestination
	mode         bufferMode
}

type bufferDestination struct {
	out     io.Writer
	written int
}

type bufferMode uint8

const (
	bufferPending bufferMode = iota
	bufferPassthrough
	bufferDiscarded
)

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.mode {
	case bufferPassthrough:
		return writeOutput(p)
	case bufferDiscarded:
		return len(p), nil
	default:
		return b.buf.Write(p)
	}
}

// Flush writes the complete buffered operation to the process log as one
// serialized record. Later writes pass through the same serialized sink.
func (b *Buffer) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.mode != bufferPending {
		return nil
	}
	if b.buf.Len() == 0 {
		b.mode = bufferPassthrough
		return nil
	}

	outputMu.Lock()
	defer outputMu.Unlock()
	if b.destinations == nil {
		b.destinations = make([]bufferDestination, len(defaultOutputs))
		for i, out := range defaultOutputs {
			b.destinations[i].out = out
		}
	}

	var errs []error
	complete := true
	for i := range b.destinations {
		destination := &b.destinations[i]
		if destination.written == b.buf.Len() {
			continue
		}
		remaining := b.buf.Bytes()[destination.written:]
		written, err := destination.out.Write(remaining)
		if written < 0 || written > len(remaining) {
			written = 0
			err = io.ErrShortWrite
		}
		destination.written += written
		if err == nil && written != len(remaining) {
			err = io.ErrShortWrite
		}
		if err != nil {
			errs = append(errs, err)
		}
		if destination.written != b.buf.Len() {
			complete = false
		}
	}
	if !complete {
		return errors.Join(errs...)
	}

	b.buf.Reset()
	b.destinations = nil
	b.mode = bufferPassthrough
	return errors.Join(errs...)
}

// Passthrough drops any unflushed remainder and routes subsequent writes to
// the process log. Callers use it after reporting an unrecoverable flush error
// so an active operation cannot remain silently buffered.
func (b *Buffer) Passthrough() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.destinations = nil
	b.mode = bufferPassthrough
}

// Discard drops the buffered operation and all late writes from its producers.
func (b *Buffer) Discard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.destinations = nil
	b.mode = bufferDiscarded
}

func writeOutput(p []byte) (int, error) {
	outputMu.Lock()
	defer outputMu.Unlock()
	return io.MultiWriter(defaultOutputs...).Write(p)
}

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

func multiWriter(out ...io.Writer) io.Writer {
	destinations := processDestinations(out...)
	if len(destinations) == 1 {
		return destinations[0]
	}
	return io.MultiWriter(destinations...)
}

func processDestinations(out ...io.Writer) []io.Writer {
	if len(out) == 0 {
		return []io.Writer{os.Stdout}
	}
	return slices.Clone(out)
}

func loggerOutput(out ...io.Writer) io.Writer {
	if len(out) == 0 {
		return processWriter{}
	}
	return &lockedWriter{out: multiWriter(out...)}
}

func NewLogger(out ...io.Writer) zerolog.Logger {
	return newLogger(loggerOutput(out...))
}

func newLogger(out io.Writer) zerolog.Logger {
	writer := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = out
		w.TimeFormat = timeFmt
		w.FormatPrepare = func(evt map[string]any) error {
			// move error field to join message if it's multiline
			if err, ok := evt[zerolog.ErrorFieldName].(string); ok {
				if strings.Count(err, "\n") == 0 {
					return nil
				}
				msg, ok := evt[zerolog.MessageFieldName].(string)
				if ok && msg != "" {
					evt[zerolog.MessageFieldName] = msg + "\n" + err
				} else {
					evt[zerolog.MessageFieldName] = err
				}
				delete(evt, zerolog.ErrorFieldName)
			}
			return nil
		}
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
		w.Out = loggerOutput(out...)
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

// NewBufferedLogger creates a synchronous logger whose complete output remains
// owned by the returned operation buffer until Flush or Discard is called.
func NewBufferedLogger(lvl zerolog.Level) (*Buffer, zerolog.Logger) {
	buf := new(Buffer)
	writer := zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = buf
		w.TimeFormat = timeFmt
		w.FormatMessage = func(msgI any) string {
			if msgI == nil {
				return ""
			}
			return fmtMessage(msgI.(string))
		}
	})
	logger := zerolog.New(writer).Level(level).With().Str("level", lvl.String()).Timestamp().Logger()
	return buf, logger
}
