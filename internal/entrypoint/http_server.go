package entrypoint

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
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

	routeEntrypointOverlays atomic.Pointer[xsync.Map[string, *routeEntrypointOverlay]]
}

type routeEntrypointOverlay struct {
	middleware          *middleware.Middleware
	consumedBypass      map[string]struct{}
	consumedMiddlewares map[string]struct{}
}

var errNoRouteEntrypointOverlay = errors.New("no route entrypoint overlay")

func newRouteEntrypointOverlayMap() *xsync.Map[string, *routeEntrypointOverlay] {
	return xsync.NewMap[string, *routeEntrypointOverlay]()
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
	srv := &httpServer{ep: ep}
	srv.resetRouteEntrypointOverlays()
	return srv
}

// Listen starts the server and stop when entrypoint is stopped.
func (srv *httpServer) Listen(addr string, proto HTTPProto) error {
	return srv.listen(addr, proto, nil)
}

func (srv *httpServer) listen(addr string, proto HTTPProto, listener net.Listener) error {
	if srv.addr != "" {
		return errors.New("server already started")
	}

	aclCfg := acl.FromCtx(srv.ep.task.Context())
	supportProxyProtocol := srv.ep.cfg.SupportProxyProtocol
	certProvider := autocert.FromCtx(srv.ep.task.Context())
	if proto == HTTPProtoHTTPS && listener == nil && certProvider != nil {
		sniListener, err := srv.ep.sni.Listen(addr)
		if err != nil {
			return err
		}
		listener = sniListener
		aclCfg = nil
		supportProxyProtocol = false
	}

	opts := server.Options{
		Name:                 addr,
		Handler:              srv,
		ACL:                  aclCfg,
		SupportProxyProtocol: supportProxyProtocol,
	}

	switch proto {
	case HTTPProtoHTTP:
		opts.HTTPAddr = addr
		opts.HTTPListener = listener
	case HTTPProtoHTTPS:
		opts.HTTPSAddr = addr
		opts.HTTPSListener = listener
		opts.CertProvider = certProvider
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
	srv.routeEntrypointOverlayMap().Delete(route.Key())
}

func (srv *httpServer) DelRoute(route types.HTTPRoute) {
	srv.routes.Del(route)
	srv.routeEntrypointOverlayMap().Delete(route.Key())
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
		entrypointMiddleware := srv.ep.middleware
		next := route.ServeHTTP
		if entrypointMiddleware != nil {
			overlay, err := srv.getRouteEntrypointOverlay(route)
			if err != nil && !errors.Is(err, errNoRouteEntrypointOverlay) {
				log.Err(err).Str("route", route.Name()).Msg("failed to compile route-specific entrypoint middleware")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if overlay != nil {
				entrypointMiddleware = overlay.middleware
				if len(overlay.consumedBypass) > 0 || len(overlay.consumedMiddlewares) > 0 {
					next = func(w http.ResponseWriter, req *http.Request) {
						route.ServeHTTP(w, middleware.WithConsumedRouteOverlays(
							req,
							overlay.consumedBypass,
							overlay.consumedMiddlewares,
						))
					}
				}
			}
		}
		if entrypointMiddleware != nil {
			entrypointMiddleware.ServeHTTP(next, w, r)
		} else {
			next(w, r)
		}
	case srv.tryHandleShortLink(w, r):
		return
	case srv.ep.notFoundHandler != nil:
		srv.ep.notFoundHandler.ServeHTTP(w, r)
	default:
		serveNotFound(w, r)
	}
}

func (srv *httpServer) getRouteEntrypointOverlay(route types.HTTPRoute) (*routeEntrypointOverlay, error) {
	if srv.ep.middleware == nil || len(srv.ep.cfg.Middlewares) == 0 {
		return nil, errNoRouteEntrypointOverlay
	}
	overlays := srv.routeEntrypointOverlayMap()
	var buildErr error
	overlay, _ := overlays.LoadOrCompute(route.Key(), func() (*routeEntrypointOverlay, bool) {
		computed, err := srv.compileRouteEntrypointOverlay(route)
		if err != nil {
			buildErr = err
			return nil, true
		}
		return computed, false
	})
	if buildErr != nil {
		return nil, buildErr
	}
	if overlay.middleware == nil {
		return nil, errNoRouteEntrypointOverlay
	}
	return overlay, nil
}

func (srv *httpServer) routeEntrypointOverlayMap() *xsync.Map[string, *routeEntrypointOverlay] {
	overlays := srv.routeEntrypointOverlays.Load()
	if overlays != nil {
		return overlays
	}
	overlays = newRouteEntrypointOverlayMap()
	if srv.routeEntrypointOverlays.CompareAndSwap(nil, overlays) {
		return overlays
	}
	return srv.routeEntrypointOverlays.Load()
}

func (srv *httpServer) resetRouteEntrypointOverlays() {
	srv.routeEntrypointOverlays.Store(newRouteEntrypointOverlayMap())
}

func (srv *httpServer) compileRouteEntrypointOverlay(route types.HTTPRoute) (*routeEntrypointOverlay, error) {
	routeMiddlewareMap := route.RouteMiddlewares()
	if len(routeMiddlewareMap) == 0 {
		return &routeEntrypointOverlay{}, nil
	}

	compiled, err := middleware.BuildEntrypointRouteOverlay(
		"entrypoint",
		srv.ep.cfg.Middlewares,
		route.Name(),
		routeMiddlewareMap,
	)
	if err != nil {
		if errors.Is(err, middleware.ErrNoEntrypointRouteOverlay) {
			return &routeEntrypointOverlay{}, nil
		}
		return nil, err
	}

	return &routeEntrypointOverlay{
		middleware:          compiled.Middleware,
		consumedBypass:      compiled.ConsumedBypass,
		consumedMiddlewares: compiled.ConsumedMiddlewares,
	}, nil
}

func (srv *httpServer) resolveRequestRoute(req *http.Request) (types.HTTPRoute, error) {
	hostRoute := srv.FindRoute(req.Host)
	// Skip per-route mTLS resolution if no TLS or a global mTLS profile is configured
	if req.TLS == nil || srv.ep.cfg.InboundMTLSProfile != "" {
		return hostRoute, nil
	}

	_, hostSecure, err := srv.resolveInboundMTLSProfileForRoute(hostRoute)
	if err != nil {
		return nil, err
	}

	serverName := req.TLS.ServerName
	if serverName == "" {
		if hostSecure {
			return nil, errSecureRouteRequiresSNI
		}
		return hostRoute, nil
	}

	sniRoute := srv.FindRoute(serverName)
	_, sniSecure, err := srv.resolveInboundMTLSProfileForRoute(sniRoute)
	if err != nil {
		return nil, err
	}
	if sniSecure {
		if !sameHTTPRoute(hostRoute, sniRoute) {
			return nil, errSecureRouteMisdirected
		}
		return sniRoute, nil
	}

	if hostSecure {
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
