package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/route/provider"
	"github.com/yusing/godoxy/internal/types"
	gperr "github.com/yusing/goutils/errs"
)

func TestRuntimeSnapshotRefreshesRecoveredProvider(t *testing.T) {
	var available atomic.Bool
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			http.Error(w, "daemon unavailable", http.StatusServiceUnavailable)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.Header().Set("API-Version", "1.47")
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte("[]"))
		case strings.HasSuffix(r.URL.Path, "/events"):
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(daemon.Close)
	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	p := provider.NewDockerProvider("recovering", types.DockerProviderConfig{URL: daemon.URL})
	state.providers.Store(p.String(), p)
	require.Error(t, p.LoadRoutes(state.Context()))
	report := configtypes.ActivationReport{Providers: state.ActivateProviders(state.Task())}
	state.setActivation(report, nil)
	initial := state.RuntimeSnapshot()
	require.Equal(t, configtypes.RuntimeDegraded, initial.Status)
	require.Equal(t, 1, initial.Activation.Providers.DegradedProviders)

	available.Store(true)
	require.Eventually(t, func() bool {
		return state.RuntimeSnapshot().Status == configtypes.RuntimeHealthy
	}, 8*time.Second, 10*time.Millisecond)
	recovered := state.RuntimeSnapshot()
	require.Equal(t, configtypes.ActivationHealthy, recovered.Health)
	require.Equal(t, 1, recovered.Activation.Providers.ReadyProviders)
	require.Zero(t, recovered.Activation.Providers.DegradedProviders)
	require.NoError(t, recovered.Activation.Providers.Providers[0].InfrastructureError)
	require.Error(t, initial.Activation.Providers.Providers[0].InfrastructureError)

	for _, component := range []string{"preparation", "diagnostics", "api", "metrics"} {
		t.Run(component, func(t *testing.T) {
			remaining := errors.New(component + " unavailable")
			var issues []configtypes.ActivationIssue
			report.API = configtypes.APIActivationReport{}
			report.Metrics = configtypes.ComponentActivation{}
			expected := configtypes.ActivationDegraded
			switch component {
			case "api":
				report.API.Main = configtypes.ComponentActivation{Configured: true, Required: true, Err: gperr.Wrap(remaining)}
				expected = configtypes.ActivationFailed
			case "metrics":
				report.Metrics = configtypes.ComponentActivation{Configured: true, Err: gperr.Wrap(remaining)}
			default:
				issues = []configtypes.ActivationIssue{{Component: component, Severity: configtypes.IssueDegraded, Err: gperr.Wrap(remaining)}}
			}
			state.setActivation(report, issues)
			snapshot := state.RuntimeSnapshot()
			require.Equal(t, expected, snapshot.Health)
			require.Equal(t, 1, snapshot.Activation.Providers.ReadyProviders)
		})
	}
	state.Task().FinishAndWait(context.Canceled)
	state.setActivation(canceledActivationReport(report, context.Canceled), []configtypes.ActivationIssue{{
		Component: "runtime", Severity: configtypes.IssueFailed, Err: gperr.Wrap(context.Canceled),
	}})
	canceled := state.RuntimeSnapshot()
	require.Equal(t, configtypes.RuntimeFailed, canceled.Status)
	require.Zero(t, canceled.Activation.Providers.ReadyProviders)
	require.Zero(t, canceled.Activation.Providers.ActiveRoutes)
	require.False(t, canceled.Activation.Providers.Providers[0].EventLoopReady)
	require.ErrorIs(t, canceled.Activation.Providers.Providers[0].InfrastructureError, context.Canceled)
	state.Stop(nil)
	require.Equal(t, configtypes.RuntimeStopping, state.RuntimeSnapshot().Status)
}
