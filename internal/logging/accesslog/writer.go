package accesslog

import (
	"io"
)

type BufferedWriter interface {
	io.Writer
	io.Closer
	Flush() error
	Resize(size int) error
}

type unbufferedWriter struct {
	w io.Writer
}

func NewUnbufferedWriter(w io.Writer) BufferedWriter {
	return unbufferedWriter{w: w}
}

func (w unbufferedWriter) Write(p []byte) (n int, err error) {
	return w.w.Write(p)
}

func (w unbufferedWriter) Close() error {
	if closer, ok := w.w.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (w unbufferedWriter) Flush() error {
	if flusher, ok := w.w.(interface{ Flush() }); ok {
		flusher.Flush()
	} else if errFlusher, ok := w.w.(interface{ FlushError() error }); ok {
		return errFlusher.FlushError()
	} else if errFlusher2, ok := w.w.(interface{ Flush() error }); ok {
		return errFlusher2.Flush()
	}
	return nil
}

func (w unbufferedWriter) Resize(size int) error {
	// No-op for unbuffered writer
	return nil
}
