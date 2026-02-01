package entrypoint

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	entrypoint "github.com/yusing/godoxy/internal/entrypoint/types"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware/errorpage"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/route/rules"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

type Entrypoint struct {
	middleware      *middleware.Middleware
	notFoundHandler http.Handler
	accessLogger    accesslog.AccessLogger
	findRouteFunc   func(host string) types.HTTPRoute
	shortLinkMatcher   *ShortLinkMatcher
}

// nil-safe
var ActiveConfig atomic.Pointer[entrypoint.Config]

func init() {
	// make sure it's not nil
	ActiveConfig.Store(&entrypoint.Config{})
}

func NewEntrypoint() Entrypoint {
	return Entrypoint{
		findRouteFunc: findRouteAnyDomain,
		shortLinkMatcher: newShortLinkMatcher(),
	}
}

func (ep *Entrypoint) ShortLinkMatcher() *ShortLinkMatcher {
	return ep.shortLinkMatcher
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
	ep.notFoundHandler = rules.BuildHandler(http.HandlerFunc(ep.serveNotFound))
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

func (ep *Entrypoint) FindRoute(s string) types.HTTPRoute {
	return ep.findRouteFunc(s)
}

func (ep *Entrypoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ep.accessLogger != nil {
		rec := accesslog.GetResponseRecorder(w)
		w = rec
		defer func() {
			ep.accessLogger.LogRequest(r, rec.Response())
			accesslog.PutResponseRecorder(rec)
		}()
	}

	route := ep.findRouteFunc(r.Host)
	switch {
	case route != nil:
		r = routes.WithRouteContext(r, route)
		if ep.middleware != nil {
			ep.middleware.ServeHTTP(route.ServeHTTP, w, r)
		} else {
			route.ServeHTTP(w, r)
		}
	case ep.tryHandleShortLink(w, r):
		return
	case ep.notFoundHandler != nil:
		ep.notFoundHandler.ServeHTTP(w, r)
	default:
		ep.serveNotFound(w, r)
	}
}

func (ep *Entrypoint) tryHandleShortLink(w http.ResponseWriter, r *http.Request) (handled bool) {
	host := r.Host
	if before, _, ok := strings.Cut(host, ":"); ok {
		host = before
	}
	if strings.EqualFold(host, common.ShortLinkPrefix) {
		if ep.middleware != nil {
			ep.middleware.ServeHTTP(ep.shortLinkMatcher.ServeHTTP, w, r)
		} else {
			ep.shortLinkMatcher.ServeHTTP(w, r)
		}
		return true
	}
	return false
}

func (ep *Entrypoint) serveNotFound(w http.ResponseWriter, r *http.Request) {
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
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := w.Write(errorPage); err != nil {
				log.Err(err).Msg("failed to write error page")
			}
		} else {
			http.NotFound(w, r)
		}
	}
}

func findRouteAnyDomain(host string) types.HTTPRoute {
	idx := strings.IndexByte(host, '.')
	if idx != -1 {
		target := host[:idx]
		if r, ok := routes.HTTP.Get(target); ok {
			return r
		}
	}
	if r, ok := routes.HTTP.Get(host); ok {
		return r
	}
	// try striping the trailing :port from the host
	if before, _, ok := strings.Cut(host, ":"); ok {
		if r, ok := routes.HTTP.Get(before); ok {
			return r
		}
	}
	return nil
}

func findRouteByDomains(domains []string) func(host string) types.HTTPRoute {
	return func(host string) types.HTTPRoute {
		host, _, _ = strings.Cut(host, ":") // strip the trailing :port
		for _, domain := range domains {
			if target, ok := strings.CutSuffix(host, domain); ok {
				if r, ok := routes.HTTP.Get(target); ok {
					return r
				}
			}
		}

		// fallback to exact match
		if r, ok := routes.HTTP.Get(host); ok {
			return r
		}
		return nil
	}
}
