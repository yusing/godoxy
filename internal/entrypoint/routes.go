package entrypoint

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/routing"
)

func (ep *Entrypoint) IterRoutes(yield func(r routing.Route) bool) {
	for _, r := range ep.HTTPRoutes().Iter {
		if !yield(r) {
			return
		}
	}
	for _, r := range ep.streamRoutes.Iter {
		if !yield(r) {
			return
		}
	}
	for _, r := range ep.excludedRoutes.Iter {
		if !yield(r) {
			return
		}
	}
}

func (ep *Entrypoint) NumRoutes() int {
	return ep.HTTPRoutes().Size() + ep.streamRoutes.Size() + ep.excludedRoutes.Size()
}

func (ep *Entrypoint) GetRoute(alias string) (routing.Route, bool) {
	if r, ok := ep.HTTPRoutes().Get(alias); ok {
		return r, true
	}
	if r, ok := ep.streamRoutes.Get(alias); ok {
		return r, true
	}
	if r, ok := ep.excludedRoutes.Get(alias); ok {
		return r, true
	}
	return nil, false
}

func (ep *Entrypoint) StartAddRoute(r routing.Route) error {
	if r.ShouldExclude() {
		ep.excludedRoutes.Add(r)
		r.Task().OnCancel("remove_route", func() {
			ep.excludedRoutes.Del(r)
		})
		return nil
	}
	switch r := r.(type) {
	case routing.HTTPRoute:
		if err := ep.AddHTTPRoute(r); err != nil {
			return err
		}
		ep.shortLinkMatcher.AddRoute(r.Key())
		r.Task().OnCancel("remove_route", func() {
			ep.delHTTPRoute(r)
			ep.shortLinkMatcher.DelRoute(r.Key())
		})
	case routing.StreamRoute:
		if asSNIRoute(r) {
			if !common.SNIRoutingForTCPRoutes {
				return fmt.Errorf("route %q listens on the shared HTTPS listener, but TCP SNI routing is disabled", r.Name())
			}
			if err := ep.sni.AddRoute(r); err != nil {
				return err
			}
			ep.streamRoutes.Add(r)
			r.Task().OnCancel("remove_sni_route", func() {
				ep.sni.DelRoute(r)
				_ = r.Stream().Close()
				ep.streamRoutes.Del(r)
			})
			return nil
		}

		err := r.ListenAndServe(r.Task().Context(), nil, nil)
		if err != nil {
			return err
		}
		ep.streamRoutes.Add(r)

		r.Task().OnCancel("remove_route", func() {
			r.Stream().Close()
			ep.streamRoutes.Del(r)
		})
	default:
		return fmt.Errorf("unknown route type: %T", r)
	}
	return nil
}

func getAddr(route routing.HTTPRoute) (httpAddr, httpsAddr string) {
	if port := route.ListenURL().Port(); port == "" || port == "0" {
		host := route.ListenURL().Hostname()
		if host == "" {
			httpAddr = common.ProxyHTTPAddr
			httpsAddr = common.ProxyHTTPSAddr
		} else {
			httpAddr = net.JoinHostPort(host, strconv.Itoa(common.ProxyHTTPPort))
			httpsAddr = net.JoinHostPort(host, strconv.Itoa(common.ProxyHTTPSPort))
		}
		return httpAddr, httpsAddr
	}

	httpsAddr = route.ListenURL().Host
	return
}

// AddHTTPRoute adds a HTTP route to the entrypoint's server.
//
// If the server does not exist, it will be created, started and return any error.
func (ep *Entrypoint) AddHTTPRoute(route routing.HTTPRoute) error {
	httpAddr, httpsAddr := getAddr(route)
	var (
		errs  []error
		added []httpRouteAddition
	)
	if httpAddr != "" {
		addition, err := ep.addHTTPRouteResult(route, httpAddr, HTTPProtoHTTP, nil)
		if err != nil {
			errs = append(errs, err)
		} else if addition.added {
			added = append(added, addition)
		}
	}
	if httpsAddr != "" {
		addition, err := ep.addHTTPRouteResult(route, httpsAddr, HTTPProtoHTTPS, nil)
		if err != nil {
			errs = append(errs, err)
		} else if addition.added {
			added = append(added, addition)
		}
	}
	if err := errors.Join(errs...); err != nil {
		ep.rollbackHTTPRouteAdditions(route, added)
		return err
	}
	return nil
}

type httpRouteAddition struct {
	addr     string
	previous routing.HTTPRoute
	added    bool
}

func (ep *Entrypoint) rollbackHTTPRouteAdditions(route routing.HTTPRoute, additions []httpRouteAddition) {
	for _, addition := range additions {
		srv, ok := ep.servers.Load(addition.addr)
		if !ok {
			continue
		}
		current, ok := srv.routes.Get(route.Key())
		if !ok || current != route {
			continue
		}
		if addition.previous != nil {
			srv.AddRoute(addition.previous)
			continue
		}
		srv.DelRoute(route)
	}
}

func (ep *Entrypoint) addHTTPRoute(route routing.HTTPRoute, addr string, proto HTTPProto) error {
	_, err := ep.addHTTPRouteResult(route, addr, proto, nil)
	return err
}

func (ep *Entrypoint) addHTTPRouteWithListener(route routing.HTTPRoute, addr string, proto HTTPProto, listener net.Listener) error {
	_, err := ep.addHTTPRouteResult(route, addr, proto, listener)
	return err
}

func (ep *Entrypoint) addHTTPRouteResult(route routing.HTTPRoute, addr string, proto HTTPProto, listener net.Listener) (httpRouteAddition, error) {
	var err error
	srv, _ := ep.servers.LoadOrCompute(addr, func() (newSrv *httpServer, cancel bool) {
		newSrv = newHTTPServer(ep)
		err = newSrv.listen(addr, proto, listener)
		cancel = err != nil
		return
	})
	if err != nil {
		return httpRouteAddition{}, err
	}
	previous, _ := srv.routes.Get(route.Key())
	if previous == route {
		return httpRouteAddition{}, nil
	}
	srv.AddRoute(route)
	current, ok := srv.routes.Get(route.Key())
	return httpRouteAddition{addr: addr, previous: previous, added: ok && current == route}, nil
}

func (ep *Entrypoint) delHTTPRoute(route routing.HTTPRoute) {
	httpAddr, httpsAddr := getAddr(route)
	if httpAddr != "" {
		srv, _ := ep.servers.Load(httpAddr)
		if srv != nil {
			srv.DelRoute(route)
		}
	}
	if httpsAddr != "" {
		srv, _ := ep.servers.Load(httpsAddr)
		if srv != nil {
			srv.DelRoute(route)
		}
	}
	// TODO: close server if no routes are left
}
