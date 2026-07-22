package config

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/autocert"
	maxmind "github.com/yusing/godoxy/internal/maxmind/types"
	"github.com/yusing/godoxy/internal/notif"
	"github.com/yusing/godoxy/internal/proxmox"
	"github.com/yusing/godoxy/internal/route"
	"github.com/yusing/godoxy/internal/route/provider"
	strutils "github.com/yusing/goutils/strings"
)

func TestStartupDiagnosticsAllowlist(t *testing.T) {
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
	require.NoError(t, routeProvider.LoadRoutes())
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
