package config

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	"github.com/yusing/godoxy/internal/common"
	configtypes "github.com/yusing/godoxy/internal/config/types"
	"github.com/yusing/godoxy/internal/health"
	iconlist "github.com/yusing/godoxy/internal/homepage/icons/list"
	"github.com/yusing/godoxy/internal/logging"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/provider"
	strutils "github.com/yusing/goutils/strings"
)

func TestBulkActivationSuppressesDuplicateRouteLogs(t *testing.T) {
	previousHTTPAddr := common.ProxyHTTPAddr
	previousHTTPSAddr := common.ProxyHTTPSAddr
	common.ProxyHTTPAddr = "127.0.0.1:0"
	common.ProxyHTTPSAddr = ""
	t.Cleanup(func() {
		common.ProxyHTTPAddr = previousHTTPAddr
		common.ProxyHTTPSAddr = previousHTTPSAddr
	})

	state := NewState()
	t.Cleanup(func() { state.Stop(nil) })
	state.Config = configtypes.DefaultConfig()
	require.NoError(t, state.initEntrypoint())

	newProvider := func(name, alias string) (*provider.Provider, *route.Route) {
		routeConfig := &route.Route{
			Scheme: route.SchemeHTTP,
			Host:   "backend.example.com",
			Port:   route.Port{Proxy: 8080},
			HealthCheck: health.HealthCheckConfig{
				Disable: true,
			},
		}
		p := provider.NewStaticProvider(name, route.Routes{
			alias: routeConfig,
		})
		require.NoError(t, p.LoadRoutes(state.Context()))
		return p, routeConfig
	}

	bulk, bulkRoute := newProvider("bulk", "bulk-route")
	state.providers.Store(bulk.String(), bulk)

	var runtimeLogs bytes.Buffer
	previousLogger := log.Logger
	log.Logger = zerolog.New(&runtimeLogs).Level(zerolog.InfoLevel)
	t.Cleanup(func() { log.Logger = previousLogger })

	report := state.ActivateProviders(state.Task())
	require.Equal(t, 1, report.ActiveRoutes)
	require.NoError(t, context.Cause(bulkRoute.Task().Context()), "successful activation must leave the route running")
	require.NotContains(t, runtimeLogs.String(), "added Bulk Route (bulk-route)")

	var inventory bytes.Buffer
	state.tmpLog = zerolog.New(&inventory).Level(zerolog.InfoLevel)
	state.printRoutesByProvider(len(bulk.String()))
	require.Contains(t, inventory.String(), "routes by provider")
	require.Contains(t, inventory.String(), "bulk-route")

	dynamic, dynamicRoute := newProvider("dynamic", "dynamic-route")
	dynamicActivation := dynamic.Activate(state.Task())
	require.Equal(t, 1, dynamicActivation.ActiveRoutes)
	require.NoError(t, context.Cause(dynamicRoute.Task().Context()), "successful dynamic activation must leave the route running")
	require.Contains(t, runtimeLogs.String(), "added Dynamic Route (dynamic-route)")

	bulkRoute.FinishAndWait(nil)
	dynamicRoute.FinishAndWait(nil)
}

func TestStartupDiagnosticsAllowlist(t *testing.T) {
	iconlist.InitCache()
	const (
		eabHMAC           = "eab-hmac-credential-sentinel"
		dnsOption         = "dns-option-credential-sentinel"
		middlewareSecret  = "middleware-credential-sentinel"
		futureSecret      = "future-credential-sentinel"
		proxmoxPassword   = "proxmox-password-sentinel"
		proxmoxToken      = "proxmox-token-sentinel"
		maxmindLicense    = "maxmind-license-sentinel"
		notificationToken = "notification-token-sentinel"
	)

	state := NewState()
	t.Cleanup(func() { state.Task().Finish(nil) })

	state.Config.AutoCert = &autocert.Config{
		EABKid:  "non-secret-account-id",
		EABHmac: eabHMAC,
		Options: map[string]strutils.Redacted{"api_key": dnsOption},
	}
	state.Config.Entrypoint.Middlewares = []map[string]any{
		{
			"oidc": map[string]any{
				"client_id":     "non-secret-client-id",
				"client_secret": middlewareSecret,
			},
		},
		{
			// Unknown future middleware shapes and malformed values must not be
			// traversed merely to render startup diagnostics.
			"future_plugin": map[any]any{"credential": futureSecret},
			"malformed":     func() {},
		},
	}
	state.Config.Providers.Proxmox = []*proxmox.Config{{
		URL:      "https://proxmox.example.com",
		Username: "operator",
		Password: proxmoxPassword,
		TokenID:  "operator-token",
		Secret:   proxmoxToken,
	}}
	state.Config.Providers.MaxMind = &maxmind.Config{
		AccountID:  "non-secret-account-id",
		LicenseKey: maxmindLicense,
	}
	state.Config.Providers.Notification = []*notif.NotificationConfig{{
		ProviderName: notif.ProviderWebhook,
		Provider: &notif.Webhook{ProviderBase: notif.ProviderBase{
			Name:  "operations",
			URL:   "https://notifications.example.com",
			Token: notificationToken,
		}},
	}}

	const providerName = "password-reset-token-service"
	routeProvider := provider.NewStaticProvider(providerName, route.Routes{
		"secret-management": {
			Scheme: route.SchemeHTTP,
			Host:   "backend.example.com",
			Port:   route.Port{Proxy: 8080},
		},
	})
	require.NoError(t, routeProvider.LoadRoutes(t.Context()))
	state.providers.Store(providerName, routeProvider)

	var diagnostics bytes.Buffer
	state.tmpLog = zerolog.New(&diagnostics).Level(zerolog.InfoLevel)
	state.logLoadedRouteProviders("loaded route providers")
	state.printRoutesByProvider(len(providerName))
	state.logStartupSummary()
	output := diagnostics.String()

	// Positive diagnostics and unrelated names containing secret-like words
	// remain visible because the output is allowlisted rather than key-filtered.
	require.Contains(t, output, "loaded route providers")
	require.Contains(t, output, providerName)
	require.Contains(t, output, "secret-management")
	require.Contains(t, output, "http://backend.example.com:8080")
	require.Contains(t, output, "entrypoint_middlewares")
	require.Contains(t, output, "maxmind")
	require.Contains(t, output, "proxmox")

	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 3)
	require.Contains(t, lines[0], "loaded route providers")
	require.Contains(t, lines[0], `"route_providers":1`)
	require.Contains(t, lines[0], `"routes":1`)
	require.NotContains(t, lines[0], "enabled_subsystems")
	require.Contains(t, lines[2], "startup configuration summary")
	require.Contains(t, lines[2], "enabled_subsystems")
	require.NotContains(t, lines[2], `"route_providers":`)
	require.NotContains(t, lines[2], `"routes":`)

	for _, secret := range []string{
		eabHMAC,
		dnsOption,
		middlewareSecret,
		futureSecret,
		proxmoxPassword,
		proxmoxToken,
		maxmindLicense,
		notificationToken,
	} {
		require.False(t, strings.Contains(output, secret), "startup diagnostics exposed %q", secret)
	}
}

func TestProxmoxDiscoveryDiagnosticsAreGroupedAndFutureCompatible(t *testing.T) {
	state := NewState()
	t.Cleanup(func() { state.Task().Finish(nil) })

	var diagnostics bytes.Buffer
	state.tmpLog = zerolog.New(&diagnostics).Level(zerolog.InfoLevel)
	state.LogProxmoxDiscoveries([]proxmox.Discovery{
		{
			Kind:   proxmox.DiscoveryResource,
			Node:   "pve-b",
			Alias:  "radarr",
			VMID:   147,
			VMName: "radarr-service",
			Target: "http://10.0.10.90:7878",
		},
		{
			Kind:   proxmox.DiscoveryNode,
			Node:   "pve-a",
			Alias:  "proxmox",
			Target: "https://10.0.0.1:8006",
		},
		{
			Kind:   "future-resource-kind",
			VMID:   42,
			VMName: "malformed\nresource",
		},
	})

	output := diagnostics.String()

	require.Equal(t, 1, strings.Count(strings.TrimSpace(output), "\n")+1, "summary must be one log record")
	require.Contains(t, output, "discovered proxmox routes")
	require.Contains(t, output, "> pve-a 1 route:")
	require.Contains(t, output, "proxmox")
	require.Contains(t, output, "node -> https://10.0.0.1:8006")
	require.Contains(t, output, "> pve-b 1 route:")
	require.Contains(t, output, "radarr-service (radarr)")
	require.Contains(t, output, "resource 147 -> http://10.0.10.90:7878")
	require.Contains(t, output, "> <unknown node> 1 route:")
	require.Contains(t, output, "<unnamed route>")
	require.Contains(t, output, "malformed resource (<unnamed route>)")
	require.Contains(t, output, "future-resource-kind 42")
	require.Less(t, strings.Index(output, "> <unknown node>"), strings.Index(output, "> pve-a"))
	require.Less(t, strings.Index(output, "> pve-a"), strings.Index(output, "> pve-b"))
	require.NotContains(t, output, "found proxmox resource")
}

func TestProxmoxDiscoveryDiagnosticsSkipEmptyWork(t *testing.T) {
	state := NewState()
	t.Cleanup(func() { state.Task().Finish(nil) })

	var diagnostics bytes.Buffer
	state.tmpLog = zerolog.New(&diagnostics).Level(zerolog.InfoLevel)
	state.LogProxmoxDiscoveries(nil)
	require.Empty(t, diagnostics.String())
}

func TestProxmoxDiscoveriesIncludeOnlySuccessfullyMarkedRoutes(t *testing.T) {
	iconlist.InitCache()
	state := NewState()
	t.Cleanup(func() { state.Task().Finish(nil) })

	successful := &route.Route{Scheme: route.SchemeHTTP, Host: "successful.example", Port: route.Port{Proxy: 80}}
	rejected := &route.Route{Scheme: route.SchemeHTTP, Host: "rejected.example", Port: route.Port{Proxy: 80}}
	provider := provider.NewStaticProvider("discoveries", route.Routes{
		"successful": successful,
		"rejected":   rejected,
	})
	require.NoError(t, provider.LoadRoutes(t.Context()))

	vmid := uint64(147)
	successful.Proxmox = &proxmox.NodeConfig{Node: "pve", VMID: &vmid, VMName: "successful-vm"}
	successful.MarkProxmoxDiscovered(proxmox.DiscoveryResource)
	rejected.Proxmox = &proxmox.NodeConfig{Node: "pve", VMID: &vmid, VMName: "rejected-vm"}
	state.providers.Store(provider.String(), provider)

	require.Equal(t, []proxmox.Discovery{{
		Kind:   proxmox.DiscoveryResource,
		Node:   "pve",
		Alias:  "successful",
		VMID:   147,
		VMName: "successful-vm",
		Target: "http://successful.example:80",
	}}, state.proxmoxDiscoveries())
}

func TestFlushTmpLogReportsPersistentFailureAndEnablesLatePassthrough(t *testing.T) {
	output := &toggleErrorWriter{err: errors.New("diagnostic sink unavailable")}
	logging.InitLogger(output)
	t.Cleanup(func() { logging.InitLogger(os.Stdout) })

	state := NewState()
	t.Cleanup(func() { state.Task().Finish(nil) })
	state.tmpLog.Info().Msg("pending diagnostics")

	require.ErrorContains(t, state.FlushTmpLog(), "diagnostic sink unavailable")
	output.err = nil
	state.tmpLog.Info().Msg("late active diagnostic")
	require.NotContains(t, output.String(), "pending diagnostics")
	require.Contains(t, output.String(), "late active diagnostic")
}

type toggleErrorWriter struct {
	bytes.Buffer
	err error
}

func (w *toggleErrorWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}
