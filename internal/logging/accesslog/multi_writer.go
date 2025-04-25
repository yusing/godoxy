package accesslog

import "strings"

type MultiWriter struct {
	writers []AccessLogIO
}

func NewMultiWriter(writers ...AccessLogIO) AccessLogIO {
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

func (w *MultiWriter) Write(p []byte) (n int, err error) {
	for _, writer := range w.writers {
		writer.Write(p)
	}
	return len(p), nil
}

func (w *MultiWriter) Lock() {
	for _, writer := range w.writers {
		writer.Lock()
	}
}

func (w *MultiWriter) Unlock() {
	for _, writer := range w.writers {
		writer.Unlock()
	}
}

func (w *MultiWriter) Name() string {
	names := make([]string, len(w.writers))
	for i, writer := range w.writers {
		names[i] = writer.Name()
	}
	return strings.Join(names, ", ")
}
