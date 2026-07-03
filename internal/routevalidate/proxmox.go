package routevalidate

import (
	"context"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	netutils "github.com/yusing/godoxy/internal/net"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	gperr "github.com/yusing/goutils/errs"
)

func validateProxmox(r *route.Route) {
	l := log.With().EmbedObject(r).Logger()

	nodeName := r.Proxmox.Node
	vmid := r.Proxmox.VMID
	if nodeName == "" || vmid == nil {
		l.Error().Msg("node (proxmox node name) is required")
		return
	}

	node, ok := proxmox.Nodes.Get(nodeName)
	if !ok {
		l.Error().Msgf("proxmox node %s not found in pool", nodeName)
		return
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
			return
		}

		r.Proxmox.VMName = res.Name

		if r.Host == route.DefaultHost {
			containerName := res.Name
			// get ip addresses of the vmid

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			ips := res.IPs
			if len(ips) == 0 {
				l.Warn().Msgf("no ip addresses found for %s, make sure you have set static ip address for container instead of dhcp", containerName)
				return
			}

			l.Info().Str("container", containerName).Msg("checking if container is running")
			running, err := node.LXCIsRunning(ctx, *vmid)
			if err != nil {
				l.Error().Err(err).Msgf("failed to check container state")
				return
			}

			if !running {
				l.Info().Msg("starting container")
				if err := node.LXCAction(ctx, *vmid, proxmox.LXCStart); err != nil {
					l.Error().Err(err).Msg("failed to start container")
					return
				}
			}

			l.Info().Msg("finding reachable ip addresses")
			errs := gperr.NewBuilder("failed to find reachable ip addresses")
			for _, ip := range ips {
				if err := netutils.PingTCP(ctx, ip, r.Port.Proxy); err != nil {
					errs.Add(gperr.Unwrap(err).Subjectf("%s:%d", ip, r.Port.Proxy))
				} else {
					r.Host = ip.String()
					l.Info().Msgf("using ip %s", r.Host)
					break
				}
			}
			if r.Host == route.DefaultHost {
				l.Warn().Err(errs.Error()).Msgf("no reachable ip addresses found, tried %d IPs", len(ips))
			}
		}
	}
}
