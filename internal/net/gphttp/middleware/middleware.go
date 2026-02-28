package middleware

import (
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/serialization"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/reverseproxy"
)

const mimeEventStream = "text/event-stream"

type (
	ReverseProxy = reverseproxy.ReverseProxy
	ProxyRequest = reverseproxy.ProxyRequest

	ImplNewFunc = func() any
	OptionsRaw  = map[string]any

	commonOptions = struct {
		// priority is only applied for ReverseProxy.
		//
		// Middleware compose follows the order of the slice
		//
		// Default is 10, 0 is the highest
		Priority int    `json:"priority"`
		Bypass   Bypass `json:"bypass"`
	}

	Middleware struct {
		commonOptions

		name      string
		construct ImplNewFunc
		impl      any
	}
	ByPriority []*Middleware

	RequestModifier interface {
		before(w http.ResponseWriter, r *http.Request) (proceed bool)
	}
	ResponseModifier             interface{ modifyResponse(r *http.Response) error }
	MiddlewareWithSetup          interface{ setup() }
	MiddlewareFinalizer          interface{ finalize() }
	MiddlewareFinalizerWithError interface {
		finalize() error
	}
)

const DefaultPriority = 10

func (m ByPriority) Len() int           { return len(m) }
func (m ByPriority) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m ByPriority) Less(i, j int) bool { return m[i].Priority < m[j].Priority }

func NewMiddleware[ImplType any]() *Middleware {
	// type check
	t := any(new(ImplType))
	switch t.(type) {
	case RequestModifier:
	case ResponseModifier:
	default:
		panic("must implement RequestModifier or ResponseModifier")
	}
	_, hasFinializer := t.(MiddlewareFinalizer)
	_, hasFinializerWithError := t.(MiddlewareFinalizerWithError)
	if hasFinializer && hasFinializerWithError {
		panic("MiddlewareFinalizer and MiddlewareFinalizerWithError are mutually exclusive")
	}
	return &Middleware{
		name:      reflect.TypeFor[ImplType]().Name(),
		construct: func() any { return new(ImplType) },
	}
}

func (m *Middleware) setup() {
	if setup, ok := m.impl.(MiddlewareWithSetup); ok {
		setup.setup()
	}
}

func (m *Middleware) apply(optsRaw OptionsRaw) error {
	if len(optsRaw) == 0 {
		return nil
	}
	commonOpts := map[string]any{}
	if priority, ok := optsRaw["priority"]; ok {
		commonOpts["priority"] = priority
	}
	if bypass, ok := optsRaw["bypass"]; ok {
		commonOpts["bypass"] = bypass
	}
	if len(commonOpts) > 0 {
		if err := serialization.MapUnmarshalValidate(commonOpts, &m.commonOptions); err != nil {
			return err
		}
		optsRaw = maps.Clone(optsRaw)
		for k := range commonOpts {
			delete(optsRaw, k)
		}
	}
	return serialization.MapUnmarshalValidate(optsRaw, m.impl)
}

func (m *Middleware) finalize() error {
	if finalizer, ok := m.impl.(MiddlewareFinalizer); ok {
		finalizer.finalize()
	}
	if finalizer, ok := m.impl.(MiddlewareFinalizerWithError); ok {
		return finalizer.finalize()
	}
	return nil
}

func (m *Middleware) New(optsRaw OptionsRaw) (*Middleware, error) {
	if m.construct == nil { // likely a middleware from compose
		if len(optsRaw) != 0 {
			return nil, fmt.Errorf("additional options not allowed for middleware %s", m.name)
		}
		return m, nil
	}
	mid := &Middleware{name: m.name, impl: m.construct()}
	mid.setup()
	if err := mid.apply(optsRaw); err != nil {
		return nil, err
	}
	if err := mid.finalize(); err != nil {
		return nil, err
	}
	mid.impl = mid.withCheckBypass()
	return mid, nil
}

func (m *Middleware) Name() string {
	return m.name
}

func (m *Middleware) String() string {
	return m.name
}

func (m *Middleware) MarshalJSON() ([]byte, error) {
	type allOptions struct {
		commonOptions
		any
	}
	return sonic.MarshalIndent(map[string]any{
		"name": m.name,
		"options": allOptions{
			commonOptions: m.commonOptions,
			any:           m.impl,
		},
	}, "", "  ")
}

func (m *Middleware) ModifyRequest(next http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	if exec, ok := m.impl.(RequestModifier); ok {
		if proceed := exec.before(w, r); !proceed {
			return
		}
	}
	next(w, r)
}

func (m *Middleware) TryModifyRequest(w http.ResponseWriter, r *http.Request) (proceed bool) {
	if exec, ok := m.impl.(RequestModifier); ok {
		return exec.before(w, r)
	}
	return true
}

func (m *Middleware) ModifyResponse(resp *http.Response) error {
	if exec, ok := m.impl.(ResponseModifier); ok {
		return exec.modifyResponse(resp)
	}
	return nil
}

func (m *Middleware) ServeHTTP(next http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	if exec, ok := m.impl.(RequestModifier); ok {
		if proceed := exec.before(w, r); !proceed {
			return
		}
	}

	if httpheaders.IsWebsocket(r.Header) || strings.Contains(strings.ToLower(r.Header.Get("Accept")), mimeEventStream) {
		next(w, r)
		return
	}

	if exec, ok := m.impl.(ResponseModifier); ok {
		rm := httputils.NewResponseModifier(w)
		sw := &ssePassthroughWriter{real: w, buf: rm}
		defer func() {
			if sw.sse {
				return // already written directly to the real writer
			}
			_, err := rm.FlushRelease()
			if err != nil {
				m.LogError(r).Err(err).Msg("failed to flush response")
			}
		}()
		next(sw, r)

		if sw.sse {
			return
		}

		currentBody := rm.BodyReader()
		currentResp := &http.Response{
			StatusCode:    rm.StatusCode(),
			Header:        rm.Header(),
			ContentLength: int64(rm.ContentLength()),
			Body:          currentBody,
			Request:       r,
		}
		allowBodyModification := canModifyResponseBody(currentResp)
		respToModify := currentResp
		if !allowBodyModification {
			shadow := *currentResp
			shadow.Body = eofReader{}
			respToModify = &shadow
		}
		if err := exec.modifyResponse(respToModify); err != nil {
			log.Err(err).Str("middleware", m.Name()).Str("url", fullURL(r)).Msg("failed to modify response")
		}

		// override the response status code
		rm.WriteHeader(respToModify.StatusCode)

		// overriding the response header
		maps.Copy(rm.Header(), respToModify.Header)

		// override the content length and body if changed
		if respToModify.Body != currentBody {
			if allowBodyModification {
				if err := rm.SetBody(respToModify.Body); err != nil {
					m.LogError(r).Err(err).Msg("failed to set response body")
				}
			} else {
				respToModify.Body.Close()
			}
		}
	} else {
		next(w, r)
	}
}

func canModifyResponseBody(resp *http.Response) bool {
	if hasNonIdentityEncoding(resp.TransferEncoding) {
		return false
	}
	if hasNonIdentityEncoding(resp.Header.Values("Transfer-Encoding")) {
		return false
	}
	if hasNonIdentityEncoding(resp.Header.Values("Content-Encoding")) {
		return false
	}
	return isTextLikeMediaType(string(httputils.GetContentType(resp.Header)))
}

func hasNonIdentityEncoding(values []string) bool {
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			if strings.TrimSpace(token) == "" || strings.EqualFold(strings.TrimSpace(token), "identity") {
				continue
			}
			return true
		}
	}
	return false
}

func isTextLikeMediaType(contentType string) bool {
	if contentType == "" {
		return false
	}
	contentType = strings.ToLower(contentType)
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	if contentType == "application/json" || strings.HasSuffix(contentType, "+json") {
		return true
	}
	if contentType == "application/xml" || strings.HasSuffix(contentType, "+xml") {
		return true
	}
	if strings.Contains(contentType, "yaml") || strings.Contains(contentType, "toml") {
		return true
	}
	if strings.Contains(contentType, "javascript") || strings.Contains(contentType, "ecmascript") {
		return true
	}
	if strings.Contains(contentType, "csv") {
		return true
	}
	return contentType == "application/x-www-form-urlencoded"
}

// ssePassthroughWriter wraps a ResponseModifier but detects SSE responses
// (Content-Type: text/event-stream) at the point headers are first committed.
// Once detected, subsequent writes go directly to the underlying ResponseWriter
// with immediate flushing, preserving real-time streaming behaviour for SSE
// endpoints that use POST (which cannot send Accept: text/event-stream upfront).
type ssePassthroughWriter struct {
	real http.ResponseWriter
	buf  *httputils.ResponseModifier
	sse  bool
}

// Header returns the buffered response headers from the ResponseModifier.
func (s *ssePassthroughWriter) Header() http.Header {
	return s.buf.Header()
}

// bypassToReal commits the status code to the buffer (so StatusCode() remains
// accurate) then copies the accumulated headers to the real ResponseWriter and
// writes the status code directly, switching all future writes to bypass mode.
func (s *ssePassthroughWriter) bypassToReal(code int) {
	s.buf.WriteHeader(code) // store code so StatusCode() is correct if caller checks
	s.sse = true
	maps.Copy(s.real.Header(), s.buf.Header())
	s.real.WriteHeader(code)
}

// WriteHeader checks whether the response is SSE before committing the status
// code. If Content-Type is text/event-stream the writer switches to passthrough
// mode; otherwise the status code is forwarded to the ResponseModifier buffer.
func (s *ssePassthroughWriter) WriteHeader(code int) {
	if strings.Contains(strings.ToLower(s.buf.Header().Get("Content-Type")), mimeEventStream) {
		s.bypassToReal(code)
		return
	}
	s.buf.WriteHeader(code)
}

// Write detects a late SSE Content-Type (set after WriteHeader was called) and
// switches to passthrough mode if needed. In passthrough mode each chunk is
// written directly to the real ResponseWriter and flushed immediately; otherwise
// the chunk is forwarded to the ResponseModifier buffer.
func (s *ssePassthroughWriter) Write(p []byte) (int, error) {
	if !s.sse && strings.Contains(strings.ToLower(s.buf.Header().Get("Content-Type")), mimeEventStream) {
		code := s.buf.StatusCode()
		if code == 0 {
			code = http.StatusOK
		}
		s.bypassToReal(code)
	}
	if s.sse {
		n, err := s.real.Write(p)
		if f, ok := s.real.(http.Flusher); ok {
			f.Flush()
		}
		return n, err
	}
	return s.buf.Write(p)
}

// Flush implements http.Flusher. It activates SSE passthrough if Content-Type
// is text/event-stream and no write has occurred yet (covering the case where a
// handler sets the header and calls Flush before the first Write). In passthrough
// mode it flushes directly to the real ResponseWriter; in buffered (non-SSE) mode
// it delegates to the ResponseModifier so middleware mutations are not bypassed.
func (s *ssePassthroughWriter) Flush() {
	if !s.sse && strings.Contains(strings.ToLower(s.buf.Header().Get("Content-Type")), mimeEventStream) {
		code := s.buf.StatusCode()
		if code == 0 {
			code = http.StatusOK
		}
		s.bypassToReal(code)
	}
	if s.sse {
		if f, ok := s.real.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	if f, ok := s.buf.(http.Flusher); ok {
		f.Flush()
	}
}

func (m *Middleware) LogWarn(req *http.Request) *zerolog.Event {
	//nolint:zerologlint
	return log.Warn().Str("middleware", m.name).
		Str("host", req.Host).
		Str("path", req.URL.Path)
}

func (m *Middleware) LogError(req *http.Request) *zerolog.Event {
	//nolint:zerologlint
	return log.Error().Str("middleware", m.name).
		Str("host", req.Host).
		Str("path", req.URL.Path)
}

func PatchReverseProxy(rp *ReverseProxy, middlewaresMap map[string]OptionsRaw) error {
	middlewares, err := compileMiddlewares(middlewaresMap)
	if err != nil {
		return err
	}
	patchReverseProxy(rp, middlewares)
	return nil
}

func patchReverseProxy(rp *ReverseProxy, middlewares []*Middleware) {
	sort.Sort(ByPriority(middlewares))

	mid := NewMiddlewareChain(rp.TargetName, middlewares)

	if before, ok := mid.impl.(RequestModifier); ok {
		next := rp.HandlerFunc
		rp.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
			if proceed := before.before(w, r); proceed {
				next(w, r)
			}
		}
	}

	if mr, ok := mid.impl.(ResponseModifier); ok {
		if rp.ModifyResponse != nil {
			ori := rp.ModifyResponse
			rp.ModifyResponse = func(res *http.Response) error {
				if err := mr.modifyResponse(res); err != nil {
					return err
				}
				return ori(res)
			}
		} else {
			rp.ModifyResponse = mr.modifyResponse
		}
	}
}
