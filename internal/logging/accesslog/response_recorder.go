package accesslog

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"sync"
)

type ResponseRecorder struct {
	w http.ResponseWriter

	resp http.Response
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
	return r
}

func PutResponseRecorder(r *ResponseRecorder) {
	r.w = nil
	r.resp = http.Response{}
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
	n, err := w.w.Write(b)
	w.resp.ContentLength += int64(n)
	return n, err
}

func (w *ResponseRecorder) WriteHeader(code int) {
	w.w.WriteHeader(code)

	if code >= http.StatusContinue && code < http.StatusOK {
		return
	}
	w.resp.StatusCode = code
}

// Hijack hijacks the connection.
func (w *ResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.w.(http.Hijacker); ok {
		return h.Hijack()
	}

	return nil, nil, fmt.Errorf("not a hijacker: %T", w.w)
}

// Flush sends any buffered data to the client.
func (w *ResponseRecorder) Flush() {
	if flusher, ok := w.w.(http.Flusher); ok {
		flusher.Flush()
	}
}
