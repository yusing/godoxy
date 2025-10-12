package rules

import (
	"bytes"
	"maps"
	"net/http"

	"github.com/yusing/goutils/synk"
)

type ResponseModifier struct {
	w          http.ResponseWriter
	buf        *bytes.Buffer
	statusCode int
	headers    http.Header
}

func GetInitResponseModifier(w http.ResponseWriter) *ResponseModifier {
	if rm, ok := w.(*ResponseModifier); ok {
		return rm
	}
	return NewResponseModifier(w)
}

func NewResponseModifier(w http.ResponseWriter) *ResponseModifier {
	return &ResponseModifier{
		w:       w,
		buf:     bytes.NewBuffer(synk.GetBytesPool().Get()),
		headers: http.Header{},
	}
}

func (rm *ResponseModifier) Unwrap() http.ResponseWriter {
	return rm.w
}

func (rm *ResponseModifier) WriteHeader(code int) {
	rm.statusCode = code
}

func (rm *ResponseModifier) Header() http.Header {
	return rm.headers
}

func (rm *ResponseModifier) Write(b []byte) (int, error) {
	return rm.buf.Write(b)
}

func (rm *ResponseModifier) FlushRelease() (int, error) {
	rm.w.WriteHeader(rm.statusCode)
	maps.Copy(rm.w.Header(), rm.headers)
	n, err := rm.w.Write(rm.buf.Bytes())
	synk.GetBytesPool().Put(rm.buf.Bytes())
	rm.buf = nil
	return n, err
}
