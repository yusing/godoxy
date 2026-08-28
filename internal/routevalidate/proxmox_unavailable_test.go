package routevalidate_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	runtimeconfig "github.com/yusing/godoxy/internal/config"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/routevalidate"
)

func TestResolveProxmoxSkipsUnavailableCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "proxmox unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	proxmoxConfig := &proxmox.Config{
		URL:     srv.URL,
		TokenID: "operator@pam!godoxy",
		Secret:  "secret",
	}
	require.Error(t, proxmoxConfig.Init(t.Context()))

	state := runtimeconfig.NewState()
	t.Cleanup(func() { state.Stop(nil) })
	state.Providers.Proxmox = []*proxmox.Config{proxmoxConfig}
	r := &route.Route{Host: "service.example.com"}

	discovery := routevalidate.ResolveProxmox(state.Context(), r)

	require.Zero(t, discovery)
	require.Nil(t, r.Proxmox)
}
