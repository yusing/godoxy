package routes

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"unsafe"

	"github.com/yusing/go-proxy/internal/types"
)

type RouteContextKey struct{}

type RouteContext struct {
	context.Context
	Route types.HTTPRoute
}

var routeContextKey = RouteContextKey{}

func (r *RouteContext) Value(key any) any {
	if key == routeContextKey {
		return r.Route
	}
	return r.Context.Value(key)
}

func WithRouteContext(r *http.Request, route types.HTTPRoute) *http.Request {
	// we don't want to copy the request object every fucking requests
	// return r.WithContext(context.WithValue(r.Context(), routeContextKey, route))
	(*requestInternal)(unsafe.Pointer(r)).ctx = &RouteContext{
		Context: r.Context(),
		Route:   route,
	}
	return r
}

func TryGetRoute(r *http.Request) types.HTTPRoute {
	if route, ok := r.Context().Value(routeContextKey).(types.HTTPRoute); ok {
		return route
	}
	return nil
}

func tryGetURL(r *http.Request) *url.URL {
	if route := TryGetRoute(r); route != nil {
		u := route.TargetURL()
		if u != nil {
			return &u.URL
		}
	}
	return nil
}

func TryGetUpstreamName(r *http.Request) string {
	if route := TryGetRoute(r); route != nil {
		return route.Name()
	}
	return ""
}

func TryGetUpstreamScheme(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Scheme
	}
	return ""
}

func TryGetUpstreamHost(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Hostname()
	}
	return ""
}

func TryGetUpstreamPort(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Port()
	}
	return ""
}

func TryGetUpstreamAddr(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.Host
	}
	return ""
}

func TryGetUpstreamURL(r *http.Request) string {
	if u := tryGetURL(r); u != nil {
		return u.String()
	}
	return ""
}

type requestInternal struct {
	Method           string
	URL              *url.URL
	Proto            string
	ProtoMajor       int
	ProtoMinor       int
	Header           http.Header
	Body             io.ReadCloser
	GetBody          func() (io.ReadCloser, error)
	ContentLength    int64
	TransferEncoding []string
	Close            bool
	Host             string
	Form             url.Values
	PostForm         url.Values
	MultipartForm    *multipart.Form
	Trailer          http.Header
	RemoteAddr       string
	RequestURI       string
	TLS              *tls.ConnectionState
	Cancel           <-chan struct{}
	Response         *http.Response
	Pattern          string
	ctx              context.Context
}

func init() {
	// make sure ctx has the same offset as http.Request
	f, ok := reflect.TypeFor[requestInternal]().FieldByName("ctx")
	if !ok {
		panic("ctx field not found")
	}
	f2, ok := reflect.TypeFor[http.Request]().FieldByName("ctx")
	if !ok {
		panic("ctx field not found")
	}
	if f.Offset != f2.Offset {
		panic(fmt.Sprintf("ctx has different offset than http.Request: %d != %d", f.Offset, f2.Offset))
	}
}
