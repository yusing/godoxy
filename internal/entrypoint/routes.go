package entrypoint

import (
	"errors"
	"net"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/types"
)

func (ep *Entrypoint) IterRoutes(yield func(r types.Route) bool) {
	for _, r := range ep.HTTPRoutes().Iter {
		if !yield(r) {
			break
		}
	}
	for _, r := range ep.streamRoutes.Iter {
		if !yield(r) {
			break
		}
	}
	for _, r := range ep.excludedRoutes.Iter {
		if !yield(r) {
			break
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

func (ep *Entrypoint) AddRoute(r types.Route) {
	if r.ShouldExclude() {
		ep.excludedRoutes.Add(r)
		r.Task().OnCancel("remove_route", func() {
			ep.excludedRoutes.Del(r)
		})
		return
	}
	switch r := r.(type) {
	case types.HTTPRoute:
		if err := ep.AddHTTPRoute(r); err != nil {
			log.Error().
				Err(err).
				Str("route", r.Key()).
				Str("listen_url", r.ListenURL().String()).
				Msg("failed to add HTTP route")
		}
		ep.shortLinkMatcher.AddRoute(r.Key())
		r.Task().OnCancel("remove_route", func() {
			ep.delHTTPRoute(r)
			ep.shortLinkMatcher.DelRoute(r.Key())
		})
	case types.StreamRoute:
		ep.streamRoutes.Add(r)
		r.Task().OnCancel("remove_route", func() {
			ep.streamRoutes.Del(r)
		})
	}
}

// AddHTTPRoute adds a HTTP route to the entrypoint's server.
//
// If the server does not exist, it will be created, started and return any error.
func (ep *Entrypoint) AddHTTPRoute(route types.HTTPRoute) error {
	if port := route.ListenURL().Port(); port == "" || port == "0" {
		host := route.ListenURL().Hostname()
		var httpAddr, httpsAddr string
		if host == "" {
			httpAddr = common.ProxyHTTPAddr
			httpsAddr = common.ProxyHTTPSAddr
		} else {
			httpAddr = net.JoinHostPort(host, strconv.Itoa(common.ProxyHTTPPort))
			httpsAddr = net.JoinHostPort(host, strconv.Itoa(common.ProxyHTTPSPort))
		}
		return errors.Join(ep.addHTTPRoute(route, httpAddr, HTTPProtoHTTP), ep.addHTTPRoute(route, httpsAddr, HTTPProtoHTTPS))
	}

	return ep.addHTTPRoute(route, route.ListenURL().Host, HTTPProtoHTTPS)
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
	addr := route.ListenURL().Host
	srv, _ := ep.servers.Load(addr)
	if srv != nil {
		srv.DelRoute(route)
	}
	// TODO: close if no servers left
}
