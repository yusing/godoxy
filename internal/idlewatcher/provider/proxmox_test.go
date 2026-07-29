package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	goproxmox "github.com/luthermonson/go-proxmox"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/proxmox"
	watcherEvents "github.com/yusing/godoxy/internal/watcher/events"
)

func TestProxmoxWatchRecoversFromTransientStatusError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/nodes/pve/lxc/101/status/current" {
			http.NotFound(w, r)
			return
		}

		switch calls.Add(1) {
		case 1:
			http.Error(w, "temporary failure", http.StatusServiceUnavailable)
		case 2:
			_, _ = io.WriteString(w, `{"data":{"status":"running"}}`)
		default:
			_, _ = io.WriteString(w, `{"data":{"status":"stopped"}}`)
		}
	}))
	t.Cleanup(server.Close)

	client := proxmox.NewClient(server.URL, goproxmox.WithHTTPClient(server.Client()))
	provider := &ProxmoxProvider{
		Node:               proxmox.NewNode(client, "pve", "node/pve"),
		vmid:               101,
		lxcName:            "app",
		stateCheckInterval: time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	eventCh, errCh := provider.Watch(ctx)

	select {
	case err, ok := <-errCh:
		require.True(t, ok, "error stream closed after a transient status failure")
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("Proxmox watcher did not report the transient status failure")
	}

	select {
	case event, ok := <-eventCh:
		require.True(t, ok, "event stream closed instead of retrying")
		require.Equal(t, watcherEvents.ActionContainerStop, event.Action)
		require.Equal(t, "101", event.ActorID)
		require.Equal(t, "app", event.ActorName)
	case <-time.After(time.Second):
		t.Fatal("Proxmox watcher did not recover after the transient status failure")
	}

	cancel()
	waitForProxmoxWatchStreamsToClose(t, eventCh, errCh)
}

func waitForProxmoxWatchStreamsToClose(
	t *testing.T,
	eventCh <-chan watcherEvents.Event,
	errCh <-chan error,
) {
	t.Helper()

	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for eventCh != nil || errCh != nil {
		select {
		case _, ok := <-eventCh:
			if !ok {
				eventCh = nil
			}
		case _, ok := <-errCh:
			if !ok {
				errCh = nil
			}
		case <-timeout.C:
			t.Fatal("Proxmox watcher streams did not close after cancellation")
		}
	}
}
