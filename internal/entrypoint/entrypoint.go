package entrypoint

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/yusing/go-proxy/internal/logging"
	gphttp "github.com/yusing/go-proxy/internal/net/gphttp"
	"github.com/yusing/go-proxy/internal/net/gphttp/accesslog"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware"
	"github.com/yusing/go-proxy/internal/net/gphttp/middleware/errorpage"
	"github.com/yusing/go-proxy/internal/route/routes"
	route "github.com/yusing/go-proxy/internal/route/types"
	"github.com/yusing/go-proxy/internal/task"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type Entrypoint struct {
	middleware    *middleware.Middleware
	accessLogger  *accesslog.AccessLogger
	findRouteFunc func(host string) (route.HTTPRoute, error)
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

	logging.Debug().Msg("entrypoint middleware loaded")
	return nil
}

func (ep *Entrypoint) SetAccessLogger(parent task.Parent, cfg *accesslog.Config) (err error) {
	if cfg == nil {
		ep.accessLogger = nil
		return
	}

	ep.accessLogger, err = accesslog.NewFileAccessLogger(parent, cfg)
	if err != nil {
		return
	}
	logging.Debug().Msg("entrypoint access logger created")
	return
}

func (ep *Entrypoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux, err := ep.findRouteFunc(r.Host)
	if err == nil {
		if ep.accessLogger != nil {
			w = gphttp.NewModifyResponseWriter(w, r, func(resp *http.Response) error {
				ep.accessLogger.Log(r, resp)
				return nil
			})
		}
		if ep.middleware != nil {
			ep.middleware.ServeHTTP(mux.ServeHTTP, w, r)
			return
		}
		mux.ServeHTTP(w, r)
		return
	}
	// Why use StatusNotFound instead of StatusBadRequest or StatusBadGateway?
	// On nginx, when route for domain does not exist, it returns StatusBadGateway.
	// Then scraper / scanners will know the subdomain is invalid.
	// With StatusNotFound, they won't know whether it's the path, or the subdomain that is invalid.
	if served := middleware.ServeStaticErrorPageFile(w, r); !served {
		logging.Err(err).
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Str("remote", r.RemoteAddr).
			Msg("request")
		errorPage, ok := errorpage.GetErrorPageByStatus(http.StatusNotFound)
		if ok {
			w.WriteHeader(http.StatusNotFound)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if _, err := w.Write(errorPage); err != nil {
				logging.Err(err).Msg("failed to write error page")
			}
		} else {
			http.Error(w, err.Error(), http.StatusNotFound)
		}
	}
}

func findRouteAnyDomain(host string) (route.HTTPRoute, error) {
	hostSplit := strutils.SplitRune(host, '.')
	target := hostSplit[0]

	if r, ok := routes.GetHTTPRouteOrExact(target, host); ok {
		return r, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoSuchRoute, target)
}

func findRouteByDomains(domains []string) func(host string) (route.HTTPRoute, error) {
	return func(host string) (route.HTTPRoute, error) {
		for _, domain := range domains {
			if strings.HasSuffix(host, domain) {
				target := strings.TrimSuffix(host, domain)
				if r, ok := routes.GetHTTPRoute(target); ok {
					return r, nil
				}
			}
		}

		// fallback to exact match
		if r, ok := routes.GetHTTPRoute(host); ok {
			return r, nil
		}
		return nil, fmt.Errorf("%w: %s", ErrNoSuchRoute, host)
	}
}
