package rules

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/synk"
)

type ResponseModifier struct {
	bufPool synk.UnsizedBytesPool

	w          http.ResponseWriter
	buf        *bytes.Buffer
	statusCode int
	shared     Cache

	origContentLength int64 // from http.Response in ResponseAsRW
	bodyModified      bool

	hijacked bool

	errs gperr.Builder
}

type Response struct {
	StatusCode int
	Header     http.Header
}

func unwrapResponseModifier(w http.ResponseWriter) *ResponseModifier {
	for {
		switch ww := w.(type) {
		case *ResponseModifier:
			return ww
		case interface{ Unwrap() http.ResponseWriter }:
			w = ww.Unwrap()
		default:
			return nil
		}
	}
}

type responseAsRW struct {
	resp *http.Response
}

func (r responseAsRW) WriteHeader(code int) {
	log.Error().Msg("write header after response has been created")
}

func (r responseAsRW) Write(b []byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func (r responseAsRW) Header() http.Header {
	return r.resp.Header
}

func ResponseAsRW(resp *http.Response) *ResponseModifier {
	return &ResponseModifier{
		statusCode:        resp.StatusCode,
		w:                 responseAsRW{resp},
		origContentLength: resp.ContentLength,
	}
}

// GetInitResponseModifier returns the response modifier for the given response writer.
// If the response writer is already wrapped, it will return the wrapped response modifier.
// Otherwise, it will return a new response modifier.
func GetInitResponseModifier(w http.ResponseWriter) *ResponseModifier {
	if rm := unwrapResponseModifier(w); rm != nil {
		return rm
	}
	return NewResponseModifier(w)
}

// GetSharedData returns the shared data for the given response writer.
// It will initialize the shared data if not initialized.
func GetSharedData(w http.ResponseWriter) Cache {
	rm := GetInitResponseModifier(w)
	if rm.shared == nil {
		rm.shared = NewCache()
	}
	return rm.shared
}

// NewResponseModifier returns a new response modifier for the given response writer.
//
// It should only be called once, at the very beginning of the request.
func NewResponseModifier(w http.ResponseWriter) *ResponseModifier {
	return &ResponseModifier{
		bufPool: synk.GetUnsizedBytesPool(),
		w:       w,
	}
}

func (rm *ResponseModifier) BufPool() synk.UnsizedBytesPool {
	return rm.bufPool
}

// func (rm *ResponseModifier) Unwrap() http.ResponseWriter {
// 	return rm.w
// }

func (rm *ResponseModifier) WriteHeader(code int) {
	rm.statusCode = code
}

// BodyReader returns a reader for the response body.
// Every call to this function will return a new reader that starts from the beginning of the buffer.
func (rm *ResponseModifier) BodyReader() io.ReadCloser {
	if rm.buf == nil {
		return io.NopCloser(bytes.NewReader(nil))
	}
	return io.NopCloser(bytes.NewReader(rm.buf.Bytes()))
}

func (rm *ResponseModifier) ResetBody() {
	if !rm.bodyModified {
		return
	}
	if rm.buf == nil {
		return
	}
	rm.buf.Reset()
}

func (rm *ResponseModifier) SetBody(r io.ReadCloser) error {
	if rm.buf == nil {
		rm.buf = rm.bufPool.GetBuffer()
	} else {
		rm.buf.Reset()
	}

	rm.bodyModified = true

	_, err := io.Copy(rm.buf, r)
	if err != nil {
		return fmt.Errorf("failed to copy body: %w", err)
	}
	r.Close()
	return nil
}

func (rm *ResponseModifier) ContentLength() int {
	if !rm.bodyModified {
		if rm.origContentLength > 0 {
			return int(rm.origContentLength)
		}
		contentLength, _ := strconv.Atoi(rm.ContentLengthStr())
		return contentLength
	}
	return rm.buf.Len()
}

func (rm *ResponseModifier) ContentLengthStr() string {
	if !rm.bodyModified {
		if rm.origContentLength > 0 {
			return strconv.FormatInt(rm.origContentLength, 10)
		}
		return rm.w.Header().Get("Content-Length")
	}
	return strconv.Itoa(rm.buf.Len())
}

func (rm *ResponseModifier) Content() []byte {
	if rm.buf == nil {
		return nil
	}
	return rm.buf.Bytes()
}

func (rm *ResponseModifier) StatusCode() int {
	if rm.statusCode == 0 {
		return http.StatusOK
	}
	return rm.statusCode
}

func (rm *ResponseModifier) Header() http.Header {
	return rm.w.Header()
}

func (rm *ResponseModifier) Response() Response {
	return Response{StatusCode: rm.StatusCode(), Header: rm.Header()}
}

func (rm *ResponseModifier) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	rm.bodyModified = true
	if rm.buf == nil {
		rm.buf = rm.bufPool.GetBuffer()
	}
	return rm.buf.Write(b)
}

// AppendError appends an error to the response modifier
// the error will be formatted as "rule <rule.Name> error: <err>"
//
// It will be aggregated and returned in FlushRelease.
func (rm *ResponseModifier) AppendError(rule Rule, err error) {
	rm.errs.Addf("rule %q error: %w", rule.Name, err)
}

func (rm *ResponseModifier) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := rm.w.(http.Hijacker); ok {
		rm.hijacked = true
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

// FlushRelease flushes the response modifier and releases the resources
// it returns the number of bytes written and the aggregated error
// if there is any error (rule errors or write error), it will be returned
func (rm *ResponseModifier) FlushRelease() (int, error) {
	n := 0
	if !rm.hijacked {
		if rm.bodyModified {
			h := rm.w.Header()
			h.Set("Content-Length", rm.ContentLengthStr())
			h.Del("Transfer-Encoding")
			h.Del("Trailer")
		}
		rm.w.WriteHeader(rm.StatusCode())

		if rm.bodyModified {
			if content := rm.Content(); len(content) > 0 {
				nn, werr := rm.w.Write(content)
				n += nn
				if werr != nil {
					rm.errs.Addf("write error: %w", werr)
				}
				if err := http.NewResponseController(rm.w).Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
					rm.errs.Addf("flush error: %w", err)
				}
			}
		}
	}

	// release the buffer and reset the pointers
	if rm.buf != nil {
		rm.bufPool.PutBuffer(rm.buf)
		rm.buf = nil
	}

	// release the shared data
	if rm.shared != nil {
		rm.shared.Release()
		rm.shared = nil
	}

	return n, rm.errs.Error()
}
