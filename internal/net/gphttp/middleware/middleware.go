package middleware

import (
	"fmt"
	"maps"
	"mime"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"encoding/json"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/serialization"
	httputils "github.com/yusing/goutils/http"
	"github.com/yusing/goutils/http/httpheaders"
	"github.com/yusing/goutils/http/reverseproxy"
)

const (
	mimeEventStream   = "text/event-stream"
	maxModifiableBody = 4 * 1024 * 1024 // 4MB
)

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
	ResponseModifier     interface{ modifyResponse(r *http.Response) error }
	BodyResponseModifier interface {
		ResponseModifier
		isBodyResponseModifier()
	}
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
	return json.MarshalIndent(map[string]any{
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

	exec, ok := m.impl.(ResponseModifier)
	if !ok {
		next(w, r)
		return
	}
	isBodyModifier := isBodyResponseModifier(exec)
	if !isBodyModifier {
		mrw := httputils.NewModifyResponseWriter(w, r, exec.modifyResponse)
		next(mrw, r)
		return
	}

	lrm := httputils.NewLazyResponseModifier(w, canBufferAndModifyResponseBody)
	lrm.SetMaxBufferedBytes(maxModifiableBody)
	defer func() {
		_, err := lrm.FlushRelease()
		if err != nil {
			m.LogError(r).Err(err).Msg("failed to flush response")
		}
	}()
	next(lrm, r)

	// Skip modification if response wasn't buffered
	if !lrm.IsBuffered() {
		return
	}

	rm := lrm.ResponseModifier()
	if rm.IsPassthrough() {
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
	respToModify := currentResp
	if err := exec.modifyResponse(respToModify); err != nil {
		log.Err(err).Str("middleware", m.Name()).Str("url", fullURL(r)).Msg("failed to modify response")
		return // skip modification if failed
	}

	// override the response status code
	rm.WriteHeader(respToModify.StatusCode)

	// overriding the response header
	maps.Copy(rm.Header(), respToModify.Header)

	// override the body if changed
	if isBodyModifier && respToModify.Body != currentBody {
		err := rm.SetBody(respToModify.Body)
		if err != nil {
			m.LogError(r).Err(err).Msg("failed to set response body")
			return // skip modification if failed
		}
	}
}

// canBufferAndModifyResponseBody checks if the response body can be buffered and modified.
//
// A body can be buffered and modified if:
// - The response is not a websocket and is not an event stream
// - The response has identity transfer encoding
// - The response has identity content encoding
// - The response has a content length
// - The content length is less than 4MB
// - The content type is text-like
func canBufferAndModifyResponseBody(respHeader http.Header) bool {
	if httpheaders.IsWebsocket(respHeader) {
		return false
	}
	contentType := respHeader.Get("Content-Type")
	if contentType == "" { // safe default: skip if no content type
		return false
	}
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, mimeEventStream) {
		return false
	}
	// strip charset or any other parameters
	contentType, _, err := mime.ParseMediaType(contentType)
	if err != nil { // skip if invalid content type
		return false
	}
	if hasNonIdentityEncoding(respHeader.Values("Content-Encoding")) {
		return false
	}
	contentLengthKnown := false
	if contentLengthRaw := respHeader.Get("Content-Length"); contentLengthRaw != "" {
		contentLength, err := strconv.ParseInt(contentLengthRaw, 10, 64)
		if err != nil || contentLength >= maxModifiableBody {
			return false
		}
		contentLengthKnown = true
	}
	if !isTextLikeMediaType(contentType) {
		return false
	}
	transferEncoding := respHeader.Values("Transfer-Encoding")
	if hasNonIdentityEncoding(transferEncoding) {
		return isHTMLLikeMediaType(contentType) && isChunkedTransferEncoding(transferEncoding)
	}
	if !contentLengthKnown {
		return isHTMLLikeMediaType(contentType)
	}
	return true
}

func hasNonIdentityEncoding(values []string) bool {
	for _, value := range values {
		for token := range strings.SplitSeq(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" || strings.EqualFold(token, "identity") {
				continue
			}
			return true
		}
	}
	return false
}

func isChunkedTransferEncoding(values []string) bool {
	foundChunked := false
	for _, value := range values {
		for token := range strings.SplitSeq(value, ",") {
			token = strings.TrimSpace(token)
			switch {
			case token == "", strings.EqualFold(token, "identity"):
				continue
			case strings.EqualFold(token, "chunked"):
				foundChunked = true
			default:
				return false
			}
		}
	}
	return foundChunked
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

func isHTMLLikeMediaType(contentType string) bool {
	return contentType == "text/html" || contentType == "application/xhtml+xml"
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
