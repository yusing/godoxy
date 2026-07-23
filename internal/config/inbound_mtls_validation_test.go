package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	runtimeconfig "github.com/yusing/godoxy/internal/config"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route"
	routeimpl "github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/types"
)

func TestRouteValidateInboundMTLSProfile(t *testing.T) {
	t.Run("rejects unknown profile", func(t *testing.T) {
		state := runtimeconfig.NewState()
		t.Cleanup(func() { state.Stop(nil) })
		*state.Value() = configtypes.Config{
			InboundMTLSProfiles: map[string]types.InboundMTLSProfile{
				"known": {UseSystemCAs: true},
			},
		}

		r := &routeimpl.Route{
			Alias:              "test",
			Scheme:             route.SchemeHTTP,
			Host:               "example.com",
			Port:               route.Port{Proxy: 80},
			InboundMTLSProfile: "missing",
		}
		err := r.ValidateContext(state.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, `inbound mTLS profile "missing" not found`)
	})

	t.Run("rejects route profile when global profile configured", func(t *testing.T) {
		state := runtimeconfig.NewState()
		t.Cleanup(func() { state.Stop(nil) })
		*state.Value() = configtypes.Config{
			InboundMTLSProfiles: map[string]types.InboundMTLSProfile{
				"corp": {UseSystemCAs: true},
			},
		}
		state.Value().Entrypoint.InboundMTLSProfile = "corp"

		r := &routeimpl.Route{
			Alias:              "test",
			Scheme:             route.SchemeHTTP,
			Host:               "example.com",
			Port:               route.Port{Proxy: 80},
			InboundMTLSProfile: "corp",
		}
		err := r.ValidateContext(state.Context())
		require.Error(t, err)
		require.ErrorContains(t, err, "route inbound_mtls_profile is not supported")
	})
}
