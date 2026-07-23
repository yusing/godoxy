package routevalidate

import (
	"testing"

	"github.com/stretchr/testify/require"
	idlewatcher "github.com/yusing/godoxy/internal/idlewatcher/runtime"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
)

func TestResolveProxmoxBindsResolvedRouteToIdlewatcher(t *testing.T) {
	vmid := uint64(119)
	r := &route.Route{
		Proxmox: &proxmox.NodeConfig{
			Node: "pve",
			VMID: &vmid,
		},
		Idlewatcher: new(idlewatcher.Config),
	}

	ResolveProxmox(t.Context(), r)

	require.Equal(t, &idlewatcher.ProxmoxConfig{
		Node: "pve",
		VMID: 119,
	}, r.Idlewatcher.Proxmox)
}

func TestResolveProxmoxUsesExplicitIdlewatcherBindingForRoute(t *testing.T) {
	r := &route.Route{
		Idlewatcher: &idlewatcher.Config{
			IdlewatcherProviderConfig: idlewatcher.ProviderConfig{
				Proxmox: &idlewatcher.ProxmoxConfig{
					Node: "pve",
					VMID: 119,
				},
			},
		},
	}

	ResolveProxmox(t.Context(), r)

	require.Equal(t, "pve", r.Proxmox.Node)
	require.Equal(t, uint64(119), *r.Proxmox.VMID)
}

func TestFailedProxmoxValidationDoesNotMarkDiscovery(t *testing.T) {
	vmid := uint64(147)
	r := &route.Route{
		Alias:   "radarr",
		Proxmox: &proxmox.NodeConfig{Node: "pve", VMID: &vmid},
	}
	if validateProxmox(t.Context(), r) {
		r.MarkProxmoxDiscovered(proxmox.DiscoveryResource)
	}

	_, ok := r.ProxmoxDiscovery()
	require.False(t, ok)
}
