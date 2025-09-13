package entrypoint

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/yusing/go-proxy/internal/logging/accesslog"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware/errorpage"
	"github.com/yusing/go-proxy/internal/route/routes"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/types"
)

type Entrypoint struct {
	middleware    *middleware.Middleware
	accessLogger  *accesslog.AccessLogger
	findRouteFunc func(host string) (types.HTTPRoute, error)
}

var ErrNoSuchRoute = errors.New("no such route")

func NewEntrypoint() *Entrypoint {
	return &Entrypoint{
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
		return
	}

	ep.accessLogger, err = accesslog.NewAccessLogger(parent, cfg)
	if err != nil {
		return
	}
	log.Debug().Msg("entrypoint access logger created")
	return
}

func (ep *Entrypoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ep.accessLogger != nil {
		w = accesslog.NewResponseRecorder(w)
		defer ep.accessLogger.Log(r, w.(*accesslog.ResponseRecorder).Response())
	}
	route, err := ep.findRouteFunc(r.Host)
	if err == nil {
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
		log.Err(err).
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("remote", r.RemoteAddr).
			Msg("request")
		errorPage, ok := errorpage.GetErrorPageByStatus(http.StatusNotFound)
		if ok {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := w.Write(errorPage); err != nil {
				log.Err(err).Msg("failed to write error page")
			}
		} else {
			http.Error(w, err.Error(), http.StatusNotFound)
		}
	}
}

func findRouteAnyDomain(host string) (types.HTTPRoute, error) {
	idx := strings.IndexByte(host, '.')
	if idx != -1 {
		target := host[:idx]
		if r, ok := routes.HTTP.Get(target); ok {
			return r, nil
		}
	}
	if r, ok := routes.HTTP.Get(host); ok {
		return r, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoSuchRoute, host)
}

func findRouteByDomains(domains []string) func(host string) (types.HTTPRoute, error) {
	return func(host string) (types.HTTPRoute, error) {
		for _, domain := range domains {
			if target, ok := strings.CutSuffix(host, domain); ok {
				if r, ok := routes.HTTP.Get(target); ok {
					return r, nil
				}
			}
		}

		// fallback to exact match
		if r, ok := routes.HTTP.Get(host); ok {
			return r, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrNoSuchRoute, host)
	}
}
