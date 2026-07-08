package entrypoint

import (
	"crypto/x509"
	"maps"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/routing"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

type HTTPRoutes interface {
	Get(alias string) (routing.HTTPRoute, bool)
}

type findRouteFunc func(HTTPRoutes, string) routing.HTTPRoute
type findRouteKeyFunc func(string, func(string) bool) (string, bool)

type Entrypoint struct {
	task *task.Task

	cfg *Config

	middleware       *middleware.Middleware
	notFoundHandler  http.Handler
	accessLogger     accesslog.AccessLogger
	findRouteFunc    findRouteFunc
	findRouteKeyFunc findRouteKeyFunc
	shortLinkMatcher *ShortLinkMatcher

	streamRoutes   *pool.Pool[routing.StreamRoute]
	excludedRoutes *pool.Pool[routing.Route]

	// this only affects future http servers creation
	httpPoolDisableLog atomic.Bool

	servers *xsync.Map[string, *httpServer] // listen addr -> server

	sni *sniRouter

	inboundMTLSProfiles map[string]*x509.CertPool
}

var _ routing.Entrypoint = &Entrypoint{}

var emptyCfg Config

func NewEntrypoint(parent task.Parent, cfg *Config) *Entrypoint {
	if cfg == nil {
		cfg = &emptyCfg
	}

	ep := &Entrypoint{
		task:                parent.Subtask("entrypoint", false),
		cfg:                 cfg,
		findRouteFunc:       findRouteAnyDomain,
		findRouteKeyFunc:    findRouteKeyAnyDomain,
		shortLinkMatcher:    newShortLinkMatcher(),
		streamRoutes:        pool.New[routing.StreamRoute]("stream_routes", "stream_routes"),
		excludedRoutes:      pool.New[routing.Route]("excluded_routes", "excluded_routes"),
		servers:             xsync.NewMap[string, *httpServer](),
		inboundMTLSProfiles: make(map[string]*x509.CertPool),
	}
	ep.sni = newSNIRouter(ep)
	return ep
}

func (ep *Entrypoint) Task() *task.Task {
	return ep.task
}

func (ep *Entrypoint) SupportProxyProtocol() bool {
	return ep.cfg.SupportProxyProtocol
}

func (ep *Entrypoint) DisablePoolsLog(v bool) {
	ep.httpPoolDisableLog.Store(v)
	// apply to all running http servers
	for _, srv := range ep.servers.Range {
		srv.routes.DisableLog(v)
	}
	// apply to other pools
	ep.streamRoutes.DisableLog(v)
	ep.excludedRoutes.DisableLog(v)
}

func (ep *Entrypoint) ShortLinkMatcher() *ShortLinkMatcher {
	return ep.shortLinkMatcher
}

func (ep *Entrypoint) HTTPRoutes() routing.PoolLike[routing.HTTPRoute] {
	return newHTTPPoolAdapter(ep)
}

func (ep *Entrypoint) StreamRoutes() routing.PoolLike[routing.StreamRoute] {
	return ep.streamRoutes
}

func (ep *Entrypoint) ExcludedRoutes() routing.RWPoolLike[routing.Route] {
	return ep.excludedRoutes
}

func (ep *Entrypoint) GetServer(addr string) (HTTPServer, bool) {
	return ep.servers.Load(addr)
}

func (ep *Entrypoint) SetFindRouteDomains(domains []string) {
	if len(domains) == 0 {
		ep.findRouteFunc = findRouteAnyDomain
		ep.findRouteKeyFunc = findRouteKeyAnyDomain
	} else {
		for i, domain := range domains {
			if !strings.HasPrefix(domain, ".") {
				domains[i] = "." + domain
			}
		}
		ep.findRouteFunc = findRouteByDomains(domains)
		ep.findRouteKeyFunc = findRouteKeyByDomains(domains)
	}
}

func (ep *Entrypoint) SetMiddlewares(mws []map[string]any) error {
	if len(mws) == 0 {
		ep.middleware = nil
		ep.cfg.Middlewares = nil
		for _, srv := range ep.servers.Range {
			srv.resetRouteEntrypointOverlays()
		}
		return nil
	}

	tmpMiddlewares := make([]map[string]any, len(mws))
	for i, mw := range mws {
		tmpMiddlewares[i] = maps.Clone(mw)
	}

	mid, err := middleware.BuildMiddlewareFromChainRaw("entrypoint", mws)
	if err != nil {
		return err
	}
	ep.middleware = mid
	ep.cfg.Middlewares = tmpMiddlewares
	for _, srv := range ep.servers.Range {
		srv.resetRouteEntrypointOverlays()
	}

	log.Debug().Msg("entrypoint middleware loaded")
	return nil
}

func (ep *Entrypoint) SetNotFoundRules(rules rules.Rules) {
	ep.notFoundHandler = rules.BuildHandler(serveNotFound)
}

func (ep *Entrypoint) SetAccessLogger(parent task.Parent, cfg *accesslog.RequestLoggerConfig) error {
	if cfg == nil {
		ep.accessLogger = nil
		return nil
	}

	accessLogger, err := accesslog.NewAccessLogger(parent, cfg)
	if err != nil {
		return err
	}

	ep.accessLogger = accessLogger
	log.Debug().Msg("entrypoint access logger created")
	return nil
}

func findRouteAnyDomain(routes HTTPRoutes, host string) routing.HTTPRoute {
	before, _, ok := strings.Cut(host, ".")
	if ok {
		target := before
		if r, ok := routes.Get(target); ok {
			return r
		}
	}
	if r, ok := routes.Get(host); ok {
		return r
	}
	// try striping the trailing :port from the host
	if before, _, ok := strings.Cut(host, ":"); ok {
		if r, ok := routes.Get(before); ok {
			return r
		}
	}
	return nil
}

func findRouteKeyAnyDomain(host string, exists func(string) bool) (string, bool) {
	before, _, ok := strings.Cut(host, ".")
	if ok && exists(before) {
		return before, true
	}
	if exists(host) {
		return host, true
	}
	// try striping the trailing :port from the host
	if before, _, ok := strings.Cut(host, ":"); ok {
		if exists(before) {
			return before, true
		}
	}
	return "", false
}

func findRouteByDomains(domains []string) func(routes HTTPRoutes, host string) routing.HTTPRoute {
	return func(routes HTTPRoutes, host string) routing.HTTPRoute {
		host, _, _ = strings.Cut(host, ":") // strip the trailing :port
		for _, domain := range domains {
			if target, ok := strings.CutSuffix(host, domain); ok {
				if r, ok := routes.Get(target); ok {
					return r
				}
			}
		}

		// fallback to exact match
		if r, ok := routes.Get(host); ok {
			return r
		}
		return nil
	}
}

func findRouteKeyByDomains(domains []string) findRouteKeyFunc {
	return func(host string, exists func(string) bool) (string, bool) {
		host, _, _ = strings.Cut(host, ":") // strip the trailing :port
		for _, domain := range domains {
			if target, ok := strings.CutSuffix(host, domain); ok && exists(target) {
				return target, true
			}
		}

		if exists(host) {
			return host, true
		}
		return "", false
	}
}
