package accesslog

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

type hijackableResponseWriter struct {
	header http.Header

	writes       int
	writeHeaders int
	flushes      int
	hijacked     bool

	conn net.Conn
	peer net.Conn
}

func newHijackableResponseWriter() *hijackableResponseWriter {
	return &hijackableResponseWriter{header: make(http.Header)}
}

func (w *hijackableResponseWriter) Header() http.Header {
	return w.header
}

func (w *hijackableResponseWriter) Write(b []byte) (int, error) {
	if w.hijacked {
		return 0, errors.New("underlying Write called after hijack")
	}
	w.writes++
	return len(b), nil
}

func (w *hijackableResponseWriter) WriteHeader(int) {
	if w.hijacked {
		panic("underlying WriteHeader called after hijack")
	}
	w.writeHeaders++
}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	w.conn, w.peer = net.Pipe()
	brw := bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn))
	return w.conn, brw, nil
}

func (w *hijackableResponseWriter) Flush() {
	if w.hijacked {
		panic("underlying Flush called after hijack")
	}
	w.flushes++
}

func TestResponseRecorderDoesNotWriteUnderlyingAfterHijack(t *testing.T) {
	underlying := newHijackableResponseWriter()
	rec := NewResponseRecorder(underlying)
	defer PutResponseRecorder(rec)

	conn, _, err := rec.Hijack()
	expect.NoError(t, err)
	defer conn.Close()
	defer underlying.peer.Close()

	expect.Equal(t, rec.Response().StatusCode, http.StatusSwitchingProtocols)

	rec.WriteHeader(http.StatusInternalServerError)
	expect.Equal(t, underlying.writeHeaders, 0)

	n, err := rec.Write([]byte("after hijack"))
	expect.ErrorIs(t, http.ErrHijacked, err)
	expect.Equal(t, n, 0)
	expect.Equal(t, underlying.writes, 0)
	expect.Equal(t, rec.Response().ContentLength, int64(0))

	expect.ErrorIs(t, http.ErrHijacked, rec.FlushError())
	expect.Equal(t, underlying.flushes, 0)
}

func TestResponseRecorderRecordsFinalStatusAndBytes(t *testing.T) {
	underlying := newHijackableResponseWriter()
	rec := NewResponseRecorder(underlying)
	defer PutResponseRecorder(rec)

	rec.WriteHeader(http.StatusEarlyHints)
	expect.Equal(t, rec.Response().StatusCode, http.StatusOK)

	n, err := rec.Write([]byte("hello"))
	expect.NoError(t, err)
	expect.Equal(t, n, 5)
	expect.Equal(t, rec.Response().StatusCode, http.StatusOK)
	expect.Equal(t, rec.Response().ContentLength, int64(5))
}
