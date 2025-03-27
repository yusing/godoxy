package middleware

import (
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/yusing/go-proxy/internal/gperr"
	"github.com/yusing/go-proxy/internal/logging"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/reverseproxy"
	"github.com/yusing/go-proxy/internal/utils"
)

type (
	Error = gperr.Error

	ReverseProxy = reverseproxy.ReverseProxy
	ProxyRequest = reverseproxy.ProxyRequest

	ImplNewFunc = func() any
	OptionsRaw  = map[string]any

	Middleware struct {
		name      string
		construct ImplNewFunc
		impl      any
		// priority is only applied for ReverseProxy.
		//
		// Middleware compose follows the order of the slice
		//
		// Default is 10, 0 is the highest
		priority int
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
	MiddlewareWithTracer interface {
		enableTrace()
		getTracer() *Tracer
	}
)

const DefaultPriority = 10

func (m ByPriority) Len() int           { return len(m) }
func (m ByPriority) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m ByPriority) Less(i, j int) bool { return m[i].priority < m[j].priority }

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
		name:      strings.ToLower(reflect.TypeFor[ImplType]().Name()),
		construct: func() any { return new(ImplType) },
	}
}

func (m *Middleware) enableTrace() {
	if tracer, ok := m.impl.(MiddlewareWithTracer); ok {
		tracer.enableTrace()
		logging.Trace().Msgf("middleware %s enabled trace", m.name)
	}
}

func (m *Middleware) getTracer() *Tracer {
	if tracer, ok := m.impl.(MiddlewareWithTracer); ok {
		return tracer.getTracer()
	}
	return nil
}

func (m *Middleware) setParent(parent *Middleware) {
	if tracer := m.getTracer(); tracer != nil {
		tracer.SetParent(parent.getTracer())
	}
}

func (m *Middleware) setup() {
	if setup, ok := m.impl.(MiddlewareWithSetup); ok {
		setup.setup()
	}
}

func (m *Middleware) apply(optsRaw OptionsRaw) gperr.Error {
	if len(optsRaw) == 0 {
		return nil
	}
	priority, ok := optsRaw["priority"].(int)
	if ok {
		m.priority = priority
		// remove priority for deserialization, restore later
		delete(optsRaw, "priority")
		defer func() {
			optsRaw["priority"] = priority
		}()
	} else {
		m.priority = DefaultPriority
	}
	return utils.Deserialize(optsRaw, m.impl)
}

func (m *Middleware) finalize() error {
	if finalizer, ok := m.impl.(MiddlewareFinalizer); ok {
		finalizer.finalize()
		return nil
	}
	if finalizer, ok := m.impl.(MiddlewareFinalizerWithError); ok {
		return finalizer.finalize()
	}
	return nil
}

func (m *Middleware) New(optsRaw OptionsRaw) (*Middleware, gperr.Error) {
	if m.construct == nil { // likely a middleware from compose
		if len(optsRaw) != 0 {
			return nil, gperr.New("additional options not allowed for middleware ").Subject(m.name)
		}
		return m, nil
	}
	mid := &Middleware{name: m.name, impl: m.construct()}
	mid.setup()
	if err := mid.apply(optsRaw); err != nil {
		return nil, err
	}
	if err := mid.finalize(); err != nil {
		return nil, gperr.Wrap(err)
	}
	return mid, nil
}

func (m *Middleware) Name() string {
	return m.name
}

func (m *Middleware) String() string {
	return m.name
}

func (m *Middleware) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"name":     m.name,
		"options":  m.impl,
		"priority": m.priority,
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

func (m *Middleware) ModifyResponse(resp *http.Response) error {
	if exec, ok := m.impl.(ResponseModifier); ok {
		return exec.modifyResponse(resp)
	}
	return nil
}

func (m *Middleware) ServeHTTP(next http.HandlerFunc, w http.ResponseWriter, r *http.Request) {
	if exec, ok := m.impl.(ResponseModifier); ok {
		w = gphttp.NewModifyResponseWriter(w, r, func(resp *http.Response) error {
			return exec.modifyResponse(resp)
		})
	}
	if exec, ok := m.impl.(RequestModifier); ok {
		if proceed := exec.before(w, r); !proceed {
			return
		}
	}
	next(w, r)
}

func PatchReverseProxy(rp *ReverseProxy, middlewaresMap map[string]OptionsRaw) (err gperr.Error) {
	var middlewares []*Middleware
	middlewares, err = compileMiddlewares(middlewaresMap)
	if err != nil {
		return
	}
	patchReverseProxy(rp, middlewares)
	return
}

func patchReverseProxy(rp *ReverseProxy, middlewares []*Middleware) {
	sort.Sort(ByPriority(middlewares))
	middlewares = append([]*Middleware{newSetUpstreamHeaders(rp)}, middlewares...)

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
