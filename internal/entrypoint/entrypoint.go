package entrypoint

import (
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/logging/accesslog"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware"
	"github.com/yusing/godoxy/internal/net/gphttp/middleware/errorpage"
	"github.com/yusing/godoxy/internal/route/routes"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

type Entrypoint struct {
	middleware      *middleware.Middleware
	accessLogger    *accesslog.AccessLogger
	findRouteFunc   func(host string) types.HTTPRoute
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
	}
}

func (ep *Entrypoint) SetFindRouteDomains(domains []string) {
	if len(domains) == 0 {
		ep.findRouteFunc = findRouteAnyDomain
	} else {
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

func (ep *Entrypoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ep.accessLogger != nil {
		w = accesslog.NewResponseRecorder(w)
		defer ep.accessLogger.Log(r, w.(*accesslog.ResponseRecorder).Response())
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
		return
	}
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
	return nil
}

func findRouteByDomains(domains []string) func(host string) types.HTTPRoute {
	return func(host string) types.HTTPRoute {
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
