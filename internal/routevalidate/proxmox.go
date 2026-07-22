package routevalidate

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	config "github.com/yusing/godoxy/internal/config/types"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	netutils "github.com/yusing/godoxy/internal/net"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	gperr "github.com/yusing/goutils/errs"
)

// ResolveProxmox applies explicit or inferred Proxmox metadata to a route and
// its idlewatcher config without contacting the Proxmox API.
func ResolveProxmox(r *route.Route) proxmox.DiscoveryKind {
	var discovery proxmox.DiscoveryKind
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
		if workingState != nil {
			proxmoxProviders = workingState.Value().Providers.Proxmox
		}
		if len(proxmoxProviders) > 0 {
			hostname := r.Host
			ip := net.ParseIP(hostname)
			for _, p := range proxmoxProviders {
				if nodeName := p.Client().ReverseLookupNode(hostname, ip, r.Alias); nodeName != "" {
					zero := uint64(0)
					if r.Proxmox == nil {
						r.Proxmox = &proxmox.NodeConfig{}
					}
					r.Proxmox.Node = nodeName
					r.Proxmox.VMID = &zero
					r.Proxmox.VMName = ""
					discovery = proxmox.DiscoveryNode
					break
				}

				resource, _ := p.Client().ReverseLookupResource(ip, hostname, r.Alias)
				if resource != nil {
					vmid := resource.VMID
					if r.Proxmox == nil {
						r.Proxmox = &proxmox.NodeConfig{}
					}
					r.Proxmox.Node = resource.Node
					r.Proxmox.VMID = &vmid
					r.Proxmox.VMName = resource.Name
					discovery = proxmox.DiscoveryResource
					break
				}
			}
		}
		if wasNotNil && (r.Proxmox.Node == "" || r.Proxmox.VMID == nil) {
			loadLogger().Warn().EmbedObject(r).Msg("no proxmox node / resource found")
		}
	}

	if r.Proxmox == nil || r.Idlewatcher == nil {
		return discovery
	}
	r.Idlewatcher.Proxmox = &idlewatcher.ProxmoxConfig{
		Node: r.Proxmox.Node,
	}
	if r.Proxmox.VMID != nil {
		r.Idlewatcher.Proxmox.VMID = *r.Proxmox.VMID
	}
	return discovery
}

func validateProxmox(r *route.Route) bool {
	l := loadLogger().With().EmbedObject(r).Logger()

	nodeName := r.Proxmox.Node
	vmid := r.Proxmox.VMID
	if nodeName == "" || vmid == nil {
		l.Error().Msg("node (proxmox node name) is required")
		return false
	}

	workingState := config.WorkingState.Load()
	if workingState == nil {
		l.Error().Msg("proxmox node pool is unavailable")
		return false
	}
	node, err := proxmox.NodeFromCtx(workingState.Context(), nodeName)
	if err != nil {
		l.Error().Err(err).Msgf("failed to resolve proxmox node %s", nodeName)
		return false
	}

	// Node-level route (VMID = 0)
	if *vmid == 0 {
		r.Scheme = route.SchemeHTTPS
		if r.Host == route.DefaultHost {
			r.Host = node.Client().BaseURL.Hostname()
		}
		port, _ := strconv.Atoi(node.Client().BaseURL.Port())
		if port == 0 {
			port = 8006
		}
		r.Port.Proxy = port
	} else {
		res, err := node.Client().GetResource("lxc", *vmid)
		if err != nil { // ErrResourceNotFound
			l.Error().Err(err).Msgf("failed to get resource %d", *vmid)
			return false
		}

		r.Proxmox.VMName = res.Name

		if r.Host == route.DefaultHost {
			containerName := res.Name
			// get ip addresses of the vmid

			ctx, cancel := context.WithTimeout(workingState.Context(), 5*time.Second)
			defer cancel()

			ips := res.IPs
			if len(ips) == 0 {
				l.Warn().Msgf("no ip addresses found for %s, make sure you have set static ip address for container instead of dhcp", containerName)
				return false
			}

			running, err := node.LXCIsRunning(ctx, *vmid)
			if err != nil {
				l.Error().Err(err).Msgf("failed to check container state")
				return false
			}

			if !running {
				l.Info().Msg("starting container")
				if err := node.LXCAction(ctx, *vmid, proxmox.LXCStart); err != nil {
					l.Error().Err(err).Msg("failed to start container")
					return false
				}
			}

			errs := gperr.NewBuilder("failed to find reachable ip addresses")
			for _, ip := range ips {
				if err := netutils.PingTCP(ctx, ip, r.Port.Proxy); err != nil {
					errs.Add(gperr.Unwrap(err).Subjectf("%s:%d", ip, r.Port.Proxy))
				} else {
					r.Host = ip.String()
					break
				}
			}
			if r.Host == route.DefaultHost {
				l.Warn().Err(errs.Error()).Msgf("no reachable ip addresses found, tried %d IPs", len(ips))
				return false
			}
		}
	}
	return true
}

func loadLogger() *zerolog.Logger {
	if workingState := config.WorkingState.Load(); workingState != nil {
		if diagnostics, ok := workingState.(config.LoadDiagnostics); ok {
			return diagnostics.LoadLogger()
		}
	}
	return &log.Logger
}
