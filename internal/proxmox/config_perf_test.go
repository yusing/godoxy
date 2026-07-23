package proxmox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

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
	node := NewNode(client, "pve", "node/pve")
	client.nodes[node.name] = node

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
	node := NewNode(client, "pve", "node/pve")
	client.nodes[node.name] = node

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

func TestUpdateResourcesPublishesUsableSnapshotWhenOneIPLookupFails(t *testing.T) {
	var failedInterfaceCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery) {
		case "/cluster/resources?type=vm":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[
				{"id":"lxc/101","name":"healthy","node":"pve","status":"running","vmid":101},
				{"id":"lxc/102","name":"broken","node":"pve","status":"running","vmid":102}
			]}`))
		case "/nodes/pve/lxc/101/interfaces?":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"eth0","inet":"10.0.0.8/24"}]}`))
		case "/nodes/pve/lxc/102/interfaces?":
			failedInterfaceCalls.Add(1)
			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)
	node := NewNode(client, "pve", "node/pve")
	client.nodes[node.name] = node

	err := client.UpdateResources(t.Context())
	require.Error(t, err)
	require.ErrorContains(t, err, "lxc/102")
	require.EqualValues(t, 1, failedInterfaceCalls.Load())

	healthy, err := client.GetResource("lxc", 101)
	require.NoError(t, err)
	require.Equal(t, "running", healthy.Status)
	require.Len(t, healthy.IPs, 1)
	require.Equal(t, []string{"10.0.0.8"}, []string{healthy.IPs[0].String()})

	broken, err := client.GetResource("lxc", 102)
	require.NoError(t, err)
	require.Equal(t, "running", broken.Status)
	require.Empty(t, broken.IPs)
}

func TestUpdateResourcesDropsCachedIPsWhenStatusChangesAndLookupFails(t *testing.T) {
	var resourcesCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch fmt.Sprintf("%s?%s", r.URL.Path, r.URL.RawQuery) {
		case "/cluster/resources?type=vm":
			status := "running"
			if resourcesCalls.Add(1) > 1 {
				status = "stopped"
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"lxc/101","name":"changing","node":"pve","status":%q,"vmid":101}]}`, status)
		case "/nodes/pve/lxc/101/interfaces?":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"name":"eth0","inet":"10.0.0.9/24"}]}`))
		case "/nodes/pve/lxc/101/config?":
			http.Error(w, "unavailable", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(srv.Client()))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)
	node := NewNode(client, "pve", "node/pve")
	client.nodes[node.name] = node

	require.NoError(t, client.UpdateResources(t.Context()))
	running, err := client.GetResource("lxc", 101)
	require.NoError(t, err)
	require.Len(t, running.IPs, 1)
	require.Equal(t, "10.0.0.9", running.IPs[0].String())

	err = client.UpdateResources(t.Context())
	require.Error(t, err)
	stopped, getErr := client.GetResource("lxc", 101)
	require.NoError(t, getErr)
	require.Equal(t, "stopped", stopped.Status)
	require.Empty(t, stopped.IPs)
	require.True(t, stopped.IPsFetchedAt.IsZero())
}

func TestUpdateResourcesRecoversAfterMalformedResponseAndPreservesFutureResource(t *testing.T) {
	var resourcesCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if resourcesCalls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"data":`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{
			"id":"future/999",
			"name":"future-resource",
			"node":"pve",
			"status":"running",
			"vmid":999
		}]}`))
	}))
	t.Cleanup(srv.Close)

	httpClient := srv.Client()
	transport := httpClient.Transport.(*http.Transport).Clone()
	transport.MaxConnsPerHost = 1
	httpClient.Transport = transport

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(httpClient))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)

	require.Error(t, client.UpdateResources(t.Context()))
	require.NoError(t, client.UpdateResources(t.Context()))
	require.EqualValues(t, 2, resourcesCalls.Load())

	resource, err := client.GetResource("future", 999)
	require.NoError(t, err)
	require.Equal(t, "future-resource", resource.Name)
	require.Empty(t, resource.IPs)
}

func TestUpdateResourcesLeavesConnectionHeadroom(t *testing.T) {
	lookupsStarted := make(chan struct{}, maxConcurrentResourceLookups)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cluster/resources":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[
				{"id":"lxc/101","node":"pve","status":"running","vmid":101},
				{"id":"lxc/102","node":"pve","status":"running","vmid":102},
				{"id":"lxc/103","node":"pve","status":"running","vmid":103}
			]}`))
		case "/nodes/pve/lxc/101/interfaces",
			"/nodes/pve/lxc/102/interfaces",
			"/nodes/pve/lxc/103/interfaces":
			lookupsStarted <- struct{}{}
			<-r.Context().Done()
		case "/probe":
			_, _ = io.WriteString(w, "ok")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	httpClient := newProxmoxHTTPClient(false)
	httpClient.Timeout = 2 * time.Second
	transport := httpClient.Transport.(*http.Transport)
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 2 * time.Second

	client := NewClient(srv.URL, goproxmox.WithHTTPClient(httpClient))
	client.Cluster = (&goproxmox.Cluster{}).New(client.Client)
	node := NewNode(client, "pve", "node/pve")
	client.nodes[node.name] = node

	updateCtx, cancelUpdate := context.WithCancel(t.Context())
	updateErr := make(chan error, 1)
	go func() {
		updateErr <- client.UpdateResources(updateCtx)
	}()

	for range maxConcurrentResourceLookups {
		select {
		case <-lookupsStarted:
		case <-time.After(time.Second):
			t.Fatal("resource lookups did not fill their concurrency budget")
		}
	}

	response, err := httpClient.Get(srv.URL + "/probe")
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "ok", string(body))

	cancelUpdate()
	select {
	case err := <-updateErr:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("resource update did not stop after cancellation")
	}
}
