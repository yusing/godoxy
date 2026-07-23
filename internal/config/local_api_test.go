package config

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
)

func TestValidateLocalAPIAddr(t *testing.T) {
	tests := []struct {
		name             string
		addr             string
		allowNonLoopback bool
		wantErr          bool
	}{
		{
			name: "localhost",
			addr: "localhost:8888",
		},
		{
			name: "ipv4_loopback",
			addr: "127.0.0.1:8888",
		},
		{
			name: "ipv6_loopback",
			addr: "[::1]:8888",
		},
		{
			name:    "all_interfaces",
			addr:    ":8888",
			wantErr: true,
		},
		{
			name:             "all_interfaces_allowed",
			addr:             ":8888",
			allowNonLoopback: true,
		},
		{
			name:    "ipv4_unspecified",
			addr:    "0.0.0.0:8888",
			wantErr: true,
		},
		{
			name:             "ipv4_unspecified_allowed",
			addr:             "0.0.0.0:8888",
			allowNonLoopback: true,
		},
		{
			name:    "lan_ip",
			addr:    "192.168.1.10:8888",
			wantErr: true,
		},
		{
			name:             "lan_ip_allowed",
			addr:             "192.168.1.10:8888",
			allowNonLoopback: true,
		},
		{
			name:    "hostname_not_loopback",
			addr:    "godoxy.internal:8888",
			wantErr: true,
		},
		{
			name:             "hostname_not_loopback_allowed",
			addr:             "godoxy.internal:8888",
			allowNonLoopback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLocalAPIAddr(tt.addr, tt.allowNonLoopback)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %q", tt.addr)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tt.addr, err)
			}
		})
	}
}

func TestActivateAPIServersAttemptsLocalWhenMainPortIsOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, occupied.Close()) })

	previousMainAddr := common.APIHTTPAddr
	previousLocalAddr := common.LocalAPIHTTPAddr
	common.APIHTTPAddr = occupied.Addr().String()
	common.LocalAPIHTTPAddr = "127.0.0.1:0"
	t.Cleanup(func() {
		common.APIHTTPAddr = previousMainAddr
		common.LocalAPIHTTPAddr = previousLocalAddr
	})

	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	report := state.ActivateAPIServers(state.Task())

	require.False(t, report.Main.Ready)
	require.Error(t, report.Main.Err)
	require.True(t, report.Main.Required)
	require.True(t, report.Local.Ready)
	require.NoError(t, report.Local.Err)
}
