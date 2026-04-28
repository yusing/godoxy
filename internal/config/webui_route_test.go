package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/provider"
	routetypes "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
)

func TestInitWebUIRouteAddsEmbeddedFileServerAndWarnsOnAliasConflict(t *testing.T) {
	state := NewState().(*state)
	state.Config = configtypes.DefaultConfig()
	state.WebUI.Aliases = []string{"godoxy"}
	t.Setenv("API_ADDR", "127.0.0.1:8888")

	conflictProvider := provider.NewStaticProvider("conflict", route.Routes{
		"godoxy": {
			Scheme: routetypes.SchemeHTTP,
			Host:   "127.0.0.1",
			Port:   routetypes.Port{Proxy: 8080},
		},
	})
	require.NoError(t, conflictProvider.LoadRoutes())
	state.providers.Store(conflictProvider.String(), conflictProvider)

	require.NoError(t, state.initWebUIRoute())

	webuiProvider, ok := state.providers.Load("webui")
	require.True(t, ok)
	r, ok := webuiProvider.GetRoute("godoxy")
	require.True(t, ok)
	fsRoute, ok := r.(types.FileServerRoute)
	require.True(t, ok)
	require.Equal(t, "embed://webui", fsRoute.RootPath())
	require.Equal(t, "webui", r.ProviderName())
	require.True(t, strings.Contains(state.tmpLogBuf.String(), "embedded webui route will be used"))
}
