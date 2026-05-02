package accesslog

import (
	"bufio"
	"net"
	"net/http"
	"sync"
)

type ResponseRecorder struct {
	w http.ResponseWriter

	resp        http.Response
	wroteHeader bool
	hijacked    bool
}

var recorderPool = sync.Pool{
	New: func() any {
		return &ResponseRecorder{}
	},
}

func GetResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	r := recorderPool.Get().(*ResponseRecorder)
	r.w = w
	r.resp = http.Response{
		StatusCode: http.StatusOK,
		Header:     w.Header(),
	}
	r.wroteHeader = false
	r.hijacked = false
	return r
}

func PutResponseRecorder(r *ResponseRecorder) {
	r.w = nil
	r.resp = http.Response{}
	r.wroteHeader = false
	r.hijacked = false
	recorderPool.Put(r)
}

func NewResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	return GetResponseRecorder(w)
}

func (w *ResponseRecorder) Unwrap() http.ResponseWriter {
	return w.w
}

func (w *ResponseRecorder) Response() *http.Response {
	return &w.resp
}

func (w *ResponseRecorder) Header() http.Header {
	return w.w.Header()
}

func (w *ResponseRecorder) Write(b []byte) (int, error) {
	if w.hijacked {
		return 0, http.ErrHijacked
	}
	if !w.wroteHeader {
		w.recordStatus(http.StatusOK)
	}
	n, err := w.w.Write(b)
	w.resp.ContentLength += int64(n)
	return n, err
}

func (w *ResponseRecorder) WriteHeader(code int) {
	if w.hijacked {
		return
	}
	w.w.WriteHeader(code)
	w.recordStatus(code)
}

func (w *ResponseRecorder) recordStatus(code int) {
	if code >= http.StatusContinue && code < http.StatusOK && code != http.StatusSwitchingProtocols {
		return
	}
	if w.wroteHeader {
		return
	}
	w.resp.StatusCode = code
	w.wroteHeader = true
}

// Hijack hijacks the connection.
func (w *ResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.w).Hijack()
	if err != nil {
		return nil, nil, err
	}
	w.hijacked = true
	if !w.wroteHeader {
		w.recordStatus(http.StatusSwitchingProtocols)
	}
	return conn, rw, nil
}

// Flush sends any buffered data to the client.
func (w *ResponseRecorder) Flush() {
	_ = w.FlushError()
}

func (w *ResponseRecorder) FlushError() error {
	if w.hijacked {
		return http.ErrHijacked
	}
	if !w.wroteHeader {
		w.recordStatus(http.StatusOK)
	}
	return http.NewResponseController(w.w).Flush()
}
