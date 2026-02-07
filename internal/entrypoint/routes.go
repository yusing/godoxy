package entrypoint

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/types"
)

func (ep *Entrypoint) IterRoutes(yield func(r types.Route) bool) {
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

func (ep *Entrypoint) GetRoute(alias string) (types.Route, bool) {
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

func (ep *Entrypoint) StartAddRoute(r types.Route) error {
	if r.ShouldExclude() {
		ep.excludedRoutes.Add(r)
		r.Task().OnCancel("remove_route", func() {
			ep.excludedRoutes.Del(r)
		})
		return nil
	}
	switch r := r.(type) {
	case types.HTTPRoute:
		if err := ep.AddHTTPRoute(r); err != nil {
			return err
		}
		ep.shortLinkMatcher.AddRoute(r.Key())
		r.Task().OnCancel("remove_route", func() {
			ep.delHTTPRoute(r)
			ep.shortLinkMatcher.DelRoute(r.Key())
		})
	case types.StreamRoute:
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

func getAddr(route types.HTTPRoute) (httpAddr, httpsAddr string) {
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
func (ep *Entrypoint) AddHTTPRoute(route types.HTTPRoute) error {
	httpAddr, httpsAddr := getAddr(route)
	var httpErr, httpsErr error
	if httpAddr != "" {
		httpErr = ep.addHTTPRoute(route, httpAddr, HTTPProtoHTTP)
	}
	if httpsAddr != "" {
		httpsErr = ep.addHTTPRoute(route, httpsAddr, HTTPProtoHTTPS)
	}
	return errors.Join(httpErr, httpsErr)
}

func (ep *Entrypoint) addHTTPRoute(route types.HTTPRoute, addr string, proto HTTPProto) error {
	var err error
	srv, _ := ep.servers.LoadOrCompute(addr, func() (newSrv *httpServer, cancel bool) {
		newSrv = newHTTPServer(ep)
		err = newSrv.Listen(addr, proto)
		cancel = err != nil
		return
	})
	if err != nil {
		return err
	}

	srv.AddRoute(route)
	return nil
}

func (ep *Entrypoint) delHTTPRoute(route types.HTTPRoute) {
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
}
