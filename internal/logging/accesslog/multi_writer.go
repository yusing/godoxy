package accesslog

import (
	"io"
	"strings"
)

type MultiWriter struct {
	writers []WriterWithName
}

type MultiWriterInterface interface {
	Unwrap() []io.Writer
}

func NewMultiWriter(writers ...WriterWithName) WriterWithName {
	if len(writers) == 0 {
		return nil
	}
	if len(writers) == 1 {
		return writers[0]
	}
	return &MultiWriter{
		writers: writers,
	}
}

func (w *MultiWriter) Unwrap() []io.Writer {
	writers := make([]io.Writer, len(w.writers))
	for i, writer := range w.writers {
		writers[i] = writer
	}
	return writers
}

func (w *MultiWriter) Write(p []byte) (n int, err error) {
	for _, writer := range w.writers {
		writer.Write(p)
	}
	return len(p), nil
}

func (w *MultiWriter) Name() string {
	names := make([]string, len(w.writers))
	for i, writer := range w.writers {
		names[i] = writer.Name()
	}
	return strings.Join(names, ", ")
}
