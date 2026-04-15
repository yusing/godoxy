package config_test

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
	config "github.com/yusing/godoxy/internal/config/types"
	entrypointtypes "github.com/yusing/godoxy/internal/entrypoint/types"
	routeimpl "github.com/yusing/godoxy/internal/route"
	route "github.com/yusing/godoxy/internal/route/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

func TestRouteValidateInboundMTLSProfile(t *testing.T) {
	prev := config.WorkingState.Load()
	t.Cleanup(func() {
		if prev != nil {
			config.WorkingState.Store(prev)
		}
	})

	t.Run("rejects unknown profile", func(t *testing.T) {
		state := &stubState{cfg: &config.Config{
			InboundMTLSProfiles: map[string]types.InboundMTLSProfile{
				"known": {UseSystemCAs: true},
			},
		}}
		config.WorkingState.Store(state)

		r := &routeimpl.Route{
			Alias:              "test",
			Scheme:             route.SchemeHTTP,
			Host:               "example.com",
			Port:               route.Port{Proxy: 80},
			InboundMTLSProfile: "missing",
		}
		err := r.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, `inbound mTLS profile "missing" not found`)
	})

	t.Run("rejects route profile when global profile configured", func(t *testing.T) {
		state := &stubState{cfg: &config.Config{
			InboundMTLSProfiles: map[string]types.InboundMTLSProfile{
				"corp": {UseSystemCAs: true},
			},
		}}
		state.cfg.Entrypoint.InboundMTLSProfile = "corp"
		config.WorkingState.Store(state)

		r := &routeimpl.Route{
			Alias:              "test",
			Scheme:             route.SchemeHTTP,
			Host:               "example.com",
			Port:               route.Port{Proxy: 80},
			InboundMTLSProfile: "corp",
		}
		err := r.Validate()
		require.Error(t, err)
		require.ErrorContains(t, err, "route inbound_mtls_profile is not supported")
	})
}

type stubState struct {
	cfg *config.Config
}

func (s *stubState) InitFromFile(string) error                 { return nil }
func (s *stubState) Init([]byte) error                         { return nil }
func (s *stubState) Task() *task.Task                          { return nil }
func (s *stubState) Context() context.Context                  { return context.Background() }
func (s *stubState) Value() *config.Config                     { return s.cfg }
func (s *stubState) Entrypoint() entrypointtypes.Entrypoint    { return nil }
func (s *stubState) ShortLinkMatcher() config.ShortLinkMatcher { return nil }
func (s *stubState) AutoCertProvider() server.CertProvider     { return nil }
func (s *stubState) LoadOrStoreProvider(string, types.RouteProvider) (types.RouteProvider, bool) {
	return nil, false
}
func (s *stubState) DeleteProvider(string) { /* no-op: test stub */ }
func (s *stubState) IterProviders() iter.Seq2[string, types.RouteProvider] {
	// no-op: returns empty iterator
	return func(func(string, types.RouteProvider) bool) {}
}
func (s *stubState) NumProviders() int     { return 0 }   // no-op: test stub
func (s *stubState) StartProviders() error { return nil } // no-op: test stub
func (s *stubState) FlushTmpLog()          { /* no-op: test stub */ }
func (s *stubState) StartAPIServers()      { /* no-op: test stub */ }
func (s *stubState) StartMetrics()         { /* no-op: test stub */ }

var _ config.State = (*stubState)(nil)
