package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	gperr "github.com/yusing/goutils/errs"
)

func TestInitialRuntimeReady(t *testing.T) {
	tests := []struct {
		name   string
		result configtypes.ReloadResult
		ready  bool
	}{
		{
			name:   "healthy committed runtime",
			result: configtypes.ReloadResult{Committed: true, Health: configtypes.ActivationHealthy},
			ready:  true,
		},
		{
			name: "degraded committed runtime retains lifecycle diagnostics",
			result: configtypes.ReloadResult{
				Committed: true,
				Health:    configtypes.ActivationDegraded,
				Issues: []configtypes.ActivationIssue{{
					Component: "provider",
					Severity:  configtypes.IssueDegraded,
					Err:       gperr.New("agent not found"),
				}},
			},
			ready: true,
		},
		{
			name:   "failed runtime",
			result: configtypes.ReloadResult{Committed: true, Health: configtypes.ActivationFailed},
		},
		{
			name:   "uncommitted runtime",
			result: configtypes.ReloadResult{Health: configtypes.ActivationHealthy},
		},
		{
			name:   "malformed zero result",
			result: configtypes.ReloadResult{},
		},
		{
			name: "unrelated issue does not override healthy committed status",
			result: configtypes.ReloadResult{
				Committed: true,
				Health:    configtypes.ActivationHealthy,
				Issues: []configtypes.ActivationIssue{{
					Component: "diagnostics",
					Severity:  configtypes.IssueDegraded,
					Err:       gperr.New("historical diagnostic"),
				}},
			},
			ready: true,
		},
		{
			name:   "unknown future health fails closed",
			result: configtypes.ReloadResult{Committed: true, Health: configtypes.ActivationHealth("future")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.ready, initialRuntimeReady(tt.result))
		})
	}
}
