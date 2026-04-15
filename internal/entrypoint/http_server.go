package entrypoint

import (
	"errors"
	"fmt"
	"net"
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

// HTTPServer is a server that listens on a given address and serves HTTP routes.
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
	return srv.listen(addr, proto, nil)
}

func (srv *httpServer) listen(addr string, proto HTTPProto, listener net.Listener) error {
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
		opts.HTTPListener = listener
	case HTTPProtoHTTPS:
		opts.HTTPSAddr = addr
		opts.HTTPSListener = listener
		opts.CertProvider = autocert.FromCtx(srv.ep.task.Context())
		opts.TLSConfigMutator = srv.mutateServerTLSConfig
	}

	task := srv.ep.task.Subtask("http_server", false)
	_, err := server.StartServer(task, opts)
	if err != nil {
		return err
	}
	srv.stopFunc = task.FinishAndWait
	srv.addr = addr
	srv.routes = pool.New[types.HTTPRoute](fmt.Sprintf("[%s] %s", proto, addr), "http_routes")
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
			// there is no body to close
			//nolint:bodyclose
			srv.ep.accessLogger.LogRequest(r, rec.Response())
			accesslog.PutResponseRecorder(rec)
		}()
	}

	route, err := srv.resolveRequestRoute(r)
	switch {
	case errors.Is(err, errSecureRouteRequiresSNI), errors.Is(err, errSecureRouteMisdirected):
		http.Error(w, err.Error(), http.StatusMisdirectedRequest)
		return
	case err != nil:
		log.Err(err).Msg("failed to resolve HTTP route")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
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

func (srv *httpServer) resolveRequestRoute(req *http.Request) (types.HTTPRoute, error) {
	hostRoute := srv.FindRoute(req.Host)
	if req.TLS == nil || srv.ep.cfg.InboundMTLSProfile != "" {
		return hostRoute, nil
	}

	hostPool, err := srv.resolveInboundMTLSProfileForRoute(hostRoute)
	if err != nil {
		return nil, err
	}

	serverName := req.TLS.ServerName
	if serverName == "" {
		if hostPool != nil {
			return nil, errSecureRouteRequiresSNI
		}
		return hostRoute, nil
	}

	sniRoute := srv.FindRoute(serverName)
	sniPool, err := srv.resolveInboundMTLSProfileForRoute(sniRoute)
	if err != nil {
		return nil, err
	}
	if sniPool != nil {
		if !sameHTTPRoute(hostRoute, sniRoute) {
			return nil, errSecureRouteMisdirected
		}
		return sniRoute, nil
	}

	if hostPool != nil {
		return nil, errSecureRouteMisdirected
	}
	return hostRoute, nil
}

func sameHTTPRoute(left, right types.HTTPRoute) bool {
	switch {
	case left == nil || right == nil:
		return left == right
	case left == right:
		return true
	default:
		return left.Key() == right.Key()
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
		log.Warn().
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
