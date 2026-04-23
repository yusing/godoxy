package proxmox

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
)

func TestLXCGetIPsWithStatusSkipsInterfacesForStoppedStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
	}{
		{
			name:   "stopped lxc skips interfaces",
			status: string(LXCStatusStopped),
		},
		{
			name:   "suspended lxc skips interfaces",
			status: string(LXCStatusSuspended),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var configCalls atomic.Int32
			var interfaceCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/nodes/pve/lxc/101/config":
					configCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"data":{"net0":"name=eth0,ip=10.0.0.5/24"}}`))
				case "/nodes/pve/lxc/101/interfaces":
					interfaceCalls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"data":[{"name":"eth0","inet":"10.0.0.8/24"}]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
			node := NewNode(client, "pve", "node/pve")

			ips, err := node.LXCGetIPsWithStatus(t.Context(), 101, tt.status)
			require.NoError(t, err)
			require.Len(t, ips, 1)
			require.Equal(t, []string{"10.0.0.5"}, []string{ips[0].String()})
			require.EqualValues(t, 1, configCalls.Load())
			require.Zero(t, interfaceCalls.Load())
		})
	}
}
func TestUpdateResourcesReusesFreshCachedIPs(t *testing.T) {
	// Not parallel: modifies global Nodes state
	Nodes.Clear()
	t.Cleanup(Nodes.Clear)

	var resourcesCalls atomic.Int32
	var interfaceCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery) {
		case "/cluster/resources?type=vm":
			resourcesCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"lxc/101","name":"demo","node":"pve","status":"running","vmid":101}]}`))
		case "/nodes/pve/lxc/101/interfaces?":
			interfaceCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"eth0","inet":"10.0.0.8/24"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)
	Nodes.Add(NewNode(client, "pve", "node/pve"))

	require.NoError(t, client.UpdateResources(t.Context()))
	require.NoError(t, client.UpdateResources(t.Context()))

	require.EqualValues(t, 2, resourcesCalls.Load())
	require.EqualValues(t, 1, interfaceCalls.Load())
	resource, err := client.GetResource("lxc", 101)
	require.NoError(t, err)
	require.Len(t, resource.IPs, 1)
	require.Equal(t, []string{"10.0.0.8"}, []string{resource.IPs[0].String()})
}

func TestUpdateResourcesRefreshesIPsWhenStatusChanges(t *testing.T) {
	Nodes.Clear()
	t.Cleanup(Nodes.Clear)

	var resourcesCalls atomic.Int32
	var configCalls atomic.Int32
	var interfaceCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery) {
		case "/cluster/resources?type=vm":
			call := resourcesCalls.Add(1)
			status := "running"
			if call > 1 {
				status = "stopped"
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"lxc/101","name":"demo","node":"pve","status":"%s","vmid":101}]}`, status)
		case "/nodes/pve/lxc/101/interfaces?":
			interfaceCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"eth0","inet":"10.0.0.8/24"}]}`))
		case "/nodes/pve/lxc/101/config?":
			configCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"net0":"name=eth0,ip=10.0.0.5/24"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)
	Nodes.Add(NewNode(client, "pve", "node/pve"))

	require.NoError(t, client.UpdateResources(t.Context()))
	require.NoError(t, client.UpdateResources(t.Context()))

	require.EqualValues(t, 2, resourcesCalls.Load())
	require.EqualValues(t, 1, interfaceCalls.Load())
	require.EqualValues(t, 1, configCalls.Load())

	resource, err := client.GetResource("lxc", 101)
	require.NoError(t, err)
	require.Equal(t, "stopped", resource.Status)
	require.Len(t, resource.IPs, 1)
	require.Equal(t, []string{"10.0.0.5"}, []string{resource.IPs[0].String()})
}
