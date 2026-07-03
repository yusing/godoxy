package routevalidate

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/common"
	config "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/entrypoint"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	netutils "github.com/yusing/godoxy/internal/net"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routeimpl"
	"github.com/yusing/godoxy/internal/routing"
	gperr "github.com/yusing/goutils/errs"
)

func Validate(r *route.Route) (impl routing.Route, agent *agentpool.Agent, err error) {
	if r.Agent != "" {
		if r.Container != nil {
			return nil, nil, errors.New("specifying agent is not allowed for docker container routes")
		}
		var ok bool
		// by agent address
		agent, ok = agentpool.Get(r.Agent)
		if !ok {
			// fallback to get agent by name
			agent, ok = agentpool.GetAgent(r.Agent)
			if !ok {
				return nil, nil, fmt.Errorf("agent %s not found", r.Agent)
			}
		}
	}

	if workingState := config.WorkingState.Load(); workingState != nil {
		cfg := workingState.Value()
		if err := entrypoint.ValidateInboundMTLSProfileRef(r.InboundMTLSProfile, cfg.Entrypoint.InboundMTLSProfile, cfg.InboundMTLSProfiles); err != nil {
			return nil, nil, err
		}
	}

	finalize(r)

	if r.InboundMTLSProfile != "" {
		switch r.Scheme {
		case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeH2C, route.SchemeFileServer:
		default:
			return nil, nil, errors.New("inbound_mtls_profile is only supported for HTTP-based routes")
		}
	}

	if r.Proxmox != nil && r.Idlewatcher != nil {
		r.Idlewatcher.Proxmox = &idlewatcher.ProxmoxConfig{
			Node: r.Proxmox.Node,
		}
		if r.Proxmox.VMID != nil {
			r.Idlewatcher.Proxmox.VMID = *r.Proxmox.VMID
		}
	}

	if r.Proxmox == nil && r.Idlewatcher != nil && r.Idlewatcher.Proxmox != nil {
		r.Proxmox = &proxmox.NodeConfig{
			Node: r.Idlewatcher.Proxmox.Node,
			VMID: &r.Idlewatcher.Proxmox.VMID,
		}
	}

	if (r.Proxmox == nil || r.Proxmox.Node == "" || r.Proxmox.VMID == nil) && r.Container == nil {
		wasNotNil := r.Proxmox != nil
		workingState := config.WorkingState.Load()
		var proxmoxProviders []*proxmox.Config
		if workingState != nil { // nil in tests
			proxmoxProviders = workingState.Value().Providers.Proxmox
		}
		if len(proxmoxProviders) > 0 {
			// it's fine if ip is nil
			hostname := r.Host
			ip := net.ParseIP(hostname)
			for _, p := range proxmoxProviders {
				// First check if hostname, IP, or alias matches a node (node-level route)
				if nodeName := p.Client().ReverseLookupNode(hostname, ip, r.Alias); nodeName != "" {
					zero := uint64(0)
					if r.Proxmox == nil {
						r.Proxmox = &proxmox.NodeConfig{}
					}
					r.Proxmox.Node = nodeName
					r.Proxmox.VMID = &zero
					r.Proxmox.VMName = ""
					log.Info().EmbedObject(r).Msg("found proxmox node")
					break
				}

				// Then check if hostname, IP, or alias matches a VM resource
				resource, _ := p.Client().ReverseLookupResource(ip, hostname, r.Alias)
				if resource != nil {
					vmid := resource.VMID
					if r.Proxmox == nil {
						r.Proxmox = &proxmox.NodeConfig{}
					}
					r.Proxmox.Node = resource.Node
					r.Proxmox.VMID = &vmid
					r.Proxmox.VMName = resource.Name
					log.Info().EmbedObject(r).Msg("found proxmox resource")
					break
				}
			}
		}
		if wasNotNil && (r.Proxmox.Node == "" || r.Proxmox.VMID == nil) {
			log.Warn().EmbedObject(r).Msg("no proxmox node / resource found")
		}
	}

	if r.Proxmox != nil {
		validateProxmox(r)
	}

	if r.Container != nil && r.Container.IdlewatcherConfig != nil {
		r.Idlewatcher = r.Container.IdlewatcherConfig
	}

	// return error if route is localhost:<godoxy_port> but route is not agent
	if !r.IsAgent() && !r.ShouldExclude() {
		switch r.Host {
		case "localhost", "127.0.0.1":
			switch r.Port.Proxy {
			case common.ProxyHTTPPort, common.ProxyHTTPSPort, common.APIHTTPPort:
				if r.Scheme.IsReverseProxy() || r.Scheme == route.SchemeTCP {
					return nil, nil, fmt.Errorf("localhost:%d is reserved for godoxy", r.Port.Proxy)
				}
			}
		}
	}

	var errs gperr.Builder
	if err := validateRules(r); err != nil {
		errs.Add(err)
	}

	if r.ShouldExclude() {
		r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s", r.Scheme, net.JoinHostPort(r.Host, strconv.Itoa(r.Port.Proxy))))
	} else {
		switch r.Scheme {
		case route.SchemeFileServer:
			r.Host = ""
			r.Port.Proxy = 0
			r.LisURL = gperr.Collect(&errs, nettypes.ParseURL, "https://"+net.JoinHostPort(r.Bind, strconv.Itoa(r.Port.Listening)))
			r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, "file://"+r.Root)
		case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeH2C:
			r.LisURL = gperr.Collect(&errs, nettypes.ParseURL, "https://"+net.JoinHostPort(r.Bind, strconv.Itoa(r.Port.Listening)))
			r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s", r.Scheme, net.JoinHostPort(r.Host, strconv.Itoa(r.Port.Proxy))))
		case route.SchemeTCP, route.SchemeUDP:
			bindIP := net.ParseIP(r.Bind)
			remoteIP := net.ParseIP(r.Host)
			toNetwork := func(ip net.IP, scheme route.Scheme) string {
				if ip == nil { // hostname, indeterminate
					return scheme.String()
				}
				if ip.To4() == nil {
					if scheme == route.SchemeTCP {
						return "tcp6"
					}
					return "udp6"
				}
				if scheme == route.SchemeTCP {
					return "tcp4"
				}
				return "udp4"
			}
			lScheme := toNetwork(bindIP, r.Scheme)
			rScheme := toNetwork(remoteIP, r.Scheme)

			r.LisURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s", lScheme, net.JoinHostPort(r.Bind, strconv.Itoa(r.Port.Listening))))
			r.ProxyURL = gperr.Collect(&errs, nettypes.ParseURL, fmt.Sprintf("%s://%s", rScheme, net.JoinHostPort(r.Host, strconv.Itoa(r.Port.Proxy))))
		}
	}

	if !r.UseHealthCheck() && (r.UseLoadBalance() || r.UseIdleWatcher()) {
		errs.Adds("cannot disable healthcheck when loadbalancer or idle watcher is enabled")
	}
	if r.RelayProxyProtocolHeader && r.Scheme != route.SchemeTCP {
		errs.Adds("relay_proxy_protocol_header is only supported for tcp routes")
	}
	if r.TLSTermination && r.Scheme != route.SchemeTCP {
		errs.Adds("tls_termination is only supported for tcp routes")
	}
	if r.TLSTermination && r.Scheme == route.SchemeTCP && r.LisURL != nil && !netutils.IsSharedHTTPSListenAddr(r.LisURL.Host) {
		errs.Adds("tls_termination is only supported on the shared HTTPS listener")
	}

	if errs.HasError() {
		return nil, nil, errs.Error()
	}

	switch r.Scheme {
	case route.SchemeFileServer:
		impl, err = routeimpl.NewFileServer(r)
	case route.SchemeHTTP, route.SchemeHTTPS, route.SchemeH2C:
		impl, err = routeimpl.NewReverseProxyRoute(r)
	case route.SchemeTCP, route.SchemeUDP:
		impl, err = routeimpl.NewStreamRoute(r)
	default:
		panic(fmt.Errorf("unexpected scheme %s for alias %s", r.Scheme, r.Alias))
	}

	if err != nil {
		return nil, nil, err
	}

	r.Excluded = r.ShouldExclude()
	if r.Excluded {
		r.ExcludedReason = r.FindExcludedReason()
	}
	return
}
