package entrypoint

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

type HTTPRoutes interface {
	Get(alias string) (types.HTTPRoute, bool)
}

type findRouteFunc func(HTTPRoutes, string) types.HTTPRoute

type Entrypoint struct {
	task *task.Task

	cfg *Config

	middleware       *middleware.Middleware
	notFoundHandler  http.Handler
	accessLogger     accesslog.AccessLogger
	findRouteFunc    findRouteFunc
	shortLinkMatcher *ShortLinkMatcher

	streamRoutes   *pool.Pool[types.StreamRoute]
	excludedRoutes *pool.Pool[types.Route]

	// this only affects future http servers creation
	httpPoolDisableLog atomic.Bool

	servers      *xsync.Map[string, *httpServer]    // listen addr -> server
	tcpListeners *xsync.Map[string, net.Listener]   // listen addr -> listener
	udpListeners *xsync.Map[string, net.PacketConn] // listen addr -> listener
}

var _ entrypoint.Entrypoint = &Entrypoint{}

var emptyCfg Config

func NewEntrypoint(parent task.Parent, cfg *Config) *Entrypoint {
	if cfg == nil {
		cfg = &emptyCfg
	}

	ep := &Entrypoint{
		task:             parent.Subtask("entrypoint", false),
		cfg:              cfg,
		findRouteFunc:    findRouteAnyDomain,
		shortLinkMatcher: newShortLinkMatcher(),
		streamRoutes:     pool.New[types.StreamRoute]("stream_routes"),
		excludedRoutes:   pool.New[types.Route]("excluded_routes"),
		servers:          xsync.NewMap[string, *httpServer](),
		tcpListeners:     xsync.NewMap[string, net.Listener](),
		udpListeners:     xsync.NewMap[string, net.PacketConn](),
	}
	ep.task.OnCancel("stop", func() {
		// servers stop on their own when context is cancelled
		var errs gperr.Group
		for _, listener := range ep.tcpListeners.Range {
			errs.Go(func() error {
				return listener.Close()
			})
		}
		for _, listener := range ep.udpListeners.Range {
			errs.Go(func() error {
				return listener.Close()
			})
		}
		if err := errs.Wait().Error(); err != nil {
			gperr.LogError("failed to stop entrypoint listeners", err)
		}
	})
	ep.task.OnFinished("cleanup", func() {
		ep.servers.Clear()
		ep.tcpListeners.Clear()
		ep.udpListeners.Clear()
	})
	return ep
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

func (ep *Entrypoint) HTTPRoutes() entrypoint.PoolLike[types.HTTPRoute] {
	return newHTTPPoolAdapter(ep)
}

func (ep *Entrypoint) StreamRoutes() entrypoint.PoolLike[types.StreamRoute] {
	return ep.streamRoutes
}

func (ep *Entrypoint) ExcludedRoutes() entrypoint.RWPoolLike[types.Route] {
	return ep.excludedRoutes
}

func (ep *Entrypoint) GetServer(addr string) (http.Handler, bool) {
	return ep.servers.Load(addr)
}

func (ep *Entrypoint) PrintServers() {
	log.Info().Msgf("servers: %v", xsync.ToPlainMap(ep.servers))
}

func (ep *Entrypoint) SetFindRouteDomains(domains []string) {
	if len(domains) == 0 {
		ep.findRouteFunc = findRouteAnyDomain
	} else {
		for i, domain := range domains {
			if !strings.HasPrefix(domain, ".") {
				domains[i] = "." + domain
			}
		}
		ep.findRouteFunc = findRouteByDomains(domains)
	}
}

func (ep *Entrypoint) SetMiddlewares(mws []map[string]any) error {
	if len(mws) == 0 {
		ep.middleware = nil
		return nil
	}

	mid, err := middleware.BuildMiddlewareFromChainRaw("entrypoint", mws)
	if err != nil {
		return err
	}
	ep.middleware = mid

	log.Debug().Msg("entrypoint middleware loaded")
	return nil
}

func (ep *Entrypoint) SetNotFoundRules(rules rules.Rules) {
	ep.notFoundHandler = rules.BuildHandler(serveNotFound)
}

func (ep *Entrypoint) SetAccessLogger(parent task.Parent, cfg *accesslog.RequestLoggerConfig) (err error) {
	if cfg == nil {
		ep.accessLogger = nil
		return err
	}

	ep.accessLogger, err = accesslog.NewAccessLogger(parent, cfg)
	if err != nil {
		return err
	}
	log.Debug().Msg("entrypoint access logger created")
	return err
}

func findRouteAnyDomain(routes HTTPRoutes, host string) types.HTTPRoute {
	idx := strings.IndexByte(host, '.')
	if idx != -1 {
		target := host[:idx]
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

func findRouteByDomains(domains []string) func(routes HTTPRoutes, host string) types.HTTPRoute {
	return func(routes HTTPRoutes, host string) types.HTTPRoute {
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
