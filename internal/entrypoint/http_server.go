package entrypoint

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	acl "github.com/yusing/godoxy/internal/acl/types"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware/errorpage"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/server"
)

// httpServer is a server that listens on a given address and serves HTTP routes.
type HTTPServer interface {
	Listen(addr string, proto HTTPProto) error
	AddRoute(route types.HTTPRoute)
	DelRoute(route types.HTTPRoute)
	FindRoute(s string) types.HTTPRoute
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

type httpServer struct {
	ep *Entrypoint

	stopFunc func(reason any)

	addr   string
	routes *pool.Pool[types.HTTPRoute]
}

type HTTPProto string

const (
	HTTPProtoHTTP  HTTPProto = "http"
	HTTPProtoHTTPS HTTPProto = "https"
)

func NewHTTPServer(ep *Entrypoint) HTTPServer {
	return newHTTPServer(ep)
}

func newHTTPServer(ep *Entrypoint) *httpServer {
	return &httpServer{ep: ep}
}

// Listen starts the server and stop when entrypoint is stopped.
func (srv *httpServer) Listen(addr string, proto HTTPProto) error {
	if srv.addr != "" {
		return errors.New("server already started")
	}

	opts := server.Options{
		Name:                 addr,
		Handler:              srv,
		ACL:                  acl.FromCtx(srv.ep.task.Context()),
		SupportProxyProtocol: srv.ep.cfg.SupportProxyProtocol,
	}

	switch proto {
	case HTTPProtoHTTP:
		opts.HTTPAddr = addr
	case HTTPProtoHTTPS:
		opts.HTTPSAddr = addr
		opts.CertProvider = autocert.FromCtx(srv.ep.task.Context())
	}

	task := srv.ep.task.Subtask("http_server", false)
	_, err := server.StartServer(task, opts)
	if err != nil {
		return err
	}
	srv.stopFunc = task.FinishAndWait
	srv.addr = addr
	srv.routes = pool.New[types.HTTPRoute](fmt.Sprintf("[%s] %s", proto, addr))
	srv.routes.DisableLog(srv.ep.httpPoolDisableLog.Load())
	return nil
}

func (srv *httpServer) Close() {
	if srv.stopFunc == nil {
		return
	}
	srv.stopFunc(nil)
}

func (srv *httpServer) AddRoute(route types.HTTPRoute) {
	srv.routes.Add(route)
}

func (srv *httpServer) DelRoute(route types.HTTPRoute) {
	srv.routes.Del(route)
}

func (srv *httpServer) FindRoute(s string) types.HTTPRoute {
	return srv.ep.findRouteFunc(srv.routes, s)
}

func (srv *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if srv.ep.accessLogger != nil {
		rec := accesslog.GetResponseRecorder(w)
		w = rec
		defer func() {
			srv.ep.accessLogger.LogRequest(r, rec.Response())
			accesslog.PutResponseRecorder(rec)
		}()
	}

	route := srv.ep.findRouteFunc(srv.routes, r.Host)
	switch {
	case route != nil:
		r = routes.WithRouteContext(r, route)
		if srv.ep.middleware != nil {
			srv.ep.middleware.ServeHTTP(route.ServeHTTP, w, r)
		} else {
			route.ServeHTTP(w, r)
		}
	case srv.tryHandleShortLink(w, r):
		return
	case srv.ep.notFoundHandler != nil:
		srv.ep.notFoundHandler.ServeHTTP(w, r)
	default:
		serveNotFound(w, r)
	}
}

func (srv *httpServer) tryHandleShortLink(w http.ResponseWriter, r *http.Request) (handled bool) {
	host := r.Host
	if before, _, ok := strings.Cut(host, ":"); ok {
		host = before
	}
	if strings.EqualFold(host, common.ShortLinkPrefix) {
		if srv.ep.middleware != nil {
			srv.ep.middleware.ServeHTTP(srv.ep.shortLinkMatcher.ServeHTTP, w, r)
		} else {
			srv.ep.shortLinkMatcher.ServeHTTP(w, r)
		}
		return true
	}
	return false
}

func serveNotFound(w http.ResponseWriter, r *http.Request) {
	// Why use StatusNotFound instead of StatusBadRequest or StatusBadGateway?
	// On nginx, when route for domain does not exist, it returns StatusBadGateway.
	// Then scraper / scanners will know the subdomain is invalid.
	// With StatusNotFound, they won't know whether it's the path, or the subdomain that is invalid.
	if served := middleware.ServeStaticErrorPageFile(w, r); !served {
		log.Error().
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("remote", r.RemoteAddr).
			Msgf("not found: %s", r.Host)
		errorPage, ok := errorpage.GetErrorPageByStatus(http.StatusNotFound)
		if ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			if _, err := w.Write(errorPage); err != nil {
				log.Err(err).Msg("failed to write error page")
			}
		} else {
			http.NotFound(w, r)
		}
	}
}
