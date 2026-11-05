package entrypoint

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

func TestResolveRequestRouteStrictSNI(t *testing.T) {
	newServer := func(t *testing.T, strict bool) *httpServer {
		t.Helper()
		ep := NewTestEntrypoint(t, &Config{StrictSNI: strict})
		ep.SetFindRouteDomains([]string{".example.com"})
		srv := newTestHTTPServer(t, ep)
		for _, alias := range []string{"app", "other", "Mixed"} {
			// Match routevalidate.finalize's canonical keys for these fake routes.
			srv.AddRoute(newFakeHTTPRoute(t, strings.ToLower(alias), ""))
		}
		return srv
	}
	newReq := func(host string, tlsState *tls.ConnectionState) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "https://"+host, nil)
		req.Host = host
		req.TLS = tlsState
		return req
	}

	tests := []struct {
		name      string
		host      string
		tls       *tls.ConnectionState
		wantRoute string
		wantErr   error
	}{
		{name: "matching sni", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "app.example.com"}, wantRoute: "app"},
		{name: "host with port", host: "app.example.com:443", tls: &tls.ConnectionState{ServerName: "app.example.com"}, wantRoute: "app"},
		{name: "sni case differs", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "APP.example.com"}, wantRoute: "app"},
		{name: "host case differs", host: "App.Example.com", tls: &tls.ConnectionState{ServerName: "app.example.com"}, wantRoute: "app"},
		{name: "host and sni case differ", host: "APP.EXAMPLE.COM", tls: &tls.ConnectionState{ServerName: "App.Example.Com"}, wantRoute: "app"},
		{name: "unknown host and sni case-variant", host: "Unknown.example.com", tls: &tls.ConnectionState{ServerName: "UNKNOWN.example.com"}},
		{name: "case-variant sni for another route", host: "App.example.com", tls: &tls.ConnectionState{ServerName: "OTHER.example.com"}, wantErr: errStrictSNIMisdirected},
		{name: "mixed-case alias", host: "Mixed.example.com", tls: &tls.ConnectionState{ServerName: "Mixed.example.com"}, wantRoute: "mixed"},
		{name: "mixed-case alias with lowercase sni", host: "Mixed.example.com", tls: &tls.ConnectionState{ServerName: "mixed.example.com"}, wantRoute: "mixed"},
		{name: "exact-case alias with ip sni", host: "Mixed.example.com", tls: &tls.ConnectionState{ServerName: "192.0.2.1"}, wantErr: errStrictSNIMisdirected},
		{name: "no sni", host: "app.example.com", tls: &tls.ConnectionState{}, wantErr: errStrictSNIRequired},
		{name: "sni for another route", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "other.example.com"}, wantErr: errStrictSNIMisdirected},
		{name: "sni matches no route", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "unknown.example.com"}, wantErr: errStrictSNIMisdirected},
		{name: "unknown host and sni fall through", host: "unknown.example.com", tls: &tls.ConnectionState{ServerName: "unknown.example.com"}},
		{name: "different unmatched names fall through", host: "unknown.example.com", tls: &tls.ConnectionState{ServerName: "missing.example.com"}},
		{name: "plain http is not checked", host: "app.example.com", wantRoute: "app"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, err := newServer(t, true).resolveRequestRoute(newReq(tt.host, tt.tls))
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, route)
				return
			}
			require.NoError(t, err)
			if tt.wantRoute == "" {
				require.Nil(t, route)
				return
			}
			require.NotNil(t, route)
			require.Equal(t, tt.wantRoute, route.Name())
		})
	}

	t.Run("disabled keeps host routing", func(t *testing.T) {
		srv := newServer(t, false)
		for _, state := range []*tls.ConnectionState{{}, {ServerName: "other.example.com"}} {
			route, err := srv.resolveRequestRoute(newReq("app.example.com", state))
			require.NoError(t, err)
			require.NotNil(t, route)
			require.Equal(t, "app", route.Name())
		}
	})
}

func TestStrictSNIHandshake(t *testing.T) {
	ca, srv, client, err := agentcert.NewAgent()
	require.NoError(t, err)
	serverCert, err := srv.ToTLSCert()
	require.NoError(t, err)
	clientCert, err := client.ToTLSCert()
	require.NoError(t, err)

	for _, mode := range []string{"open", "global mtls", "route mtls"} {
		t.Run(mode, func(t *testing.T) {
			cfg := &Config{StrictSNI: true}
			if mode == "global mtls" {
				cfg.InboundMTLSProfile = "clients"
			}
			ep := NewTestEntrypoint(t, cfg)
			t.Cleanup(func() { closeTestServers(t, ep) })
			ep.SetFindRouteDomains([]string{".ExAmPlE.CoM"})
			autocert.SetCtx(task.GetTestTask(t), &staticCertProvider{cert: serverCert})
			insecure := &tls.Config{InsecureSkipVerify: true}
			profile := ""
			if mode != "open" {
				require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
					"clients": {CAFiles: []string{writeTempFile(t, "ca.pem", ca.Cert)}},
				}))
				insecure.Certificates = []tls.Certificate{*clientCert}
				if mode == "route mtls" {
					profile = "clients"
				}
			}
			listener, releaseListener := reserveTCPAddr(t)
			listenAddr := listener.Addr().String()
			for i, alias := range []string{"app", "other", "Mixed", "api.example.net", "*.example.com"} {
				// Production finalization canonicalizes mixed-case aliases.
				alias = strings.ToLower(alias)
				route := newFakeHTTPRouteAt(t, alias, profile, "https://"+listenAddr)
				route.handler = func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, alias) }
				if i == 0 {
					require.NoError(t, ep.addHTTPRouteWithListener(route, listenAddr, HTTPProtoHTTPS, listener))
					releaseListener()
				} else {
					require.NoError(t, ep.StartAddRoute(route))
				}
			}
			for _, tc := range []struct {
				name, host, serverName, body string
				status                       int
			}{
				{name: "matching exact", host: "app.example.com", serverName: "app.example.com", status: http.StatusOK, body: "app"},
				{name: "exact before wildcard", host: "APP.EXAMPLE.COM:443", serverName: "app.example.com", status: http.StatusOK, body: "app"},
				{name: "fqdn exact", host: "API.EXAMPLE.NET", serverName: "api.example.net", status: http.StatusOK, body: "api.example.net"},
				{name: "mixed alias", host: "mixed.example.com", serverName: "Mixed.example.com", status: http.StatusOK, body: "mixed"},
				{name: "same route different names", host: "app", serverName: "app.example.com", status: http.StatusOK, body: "app"},
				{name: "same wildcard different names", host: "one.example.com", serverName: "two.example.com", status: http.StatusOK, body: "*.example.com"},
				{name: "exact vs wildcard", host: "one.example.com", serverName: "app.example.com", status: http.StatusMisdirectedRequest},
				{name: "no sni", host: "app.example.com", status: http.StatusMisdirectedRequest},
				{name: "ip as sni", host: "app.example.com", serverName: "127.0.0.1", status: http.StatusMisdirectedRequest},
				{name: "distinct routes", host: "app.example.com", serverName: "other.example.com", status: http.StatusMisdirectedRequest},
				{name: "unmatched sni", host: "app.example.com", serverName: "unknown.invalid", status: http.StatusMisdirectedRequest},
				{name: "unmatched host", host: "unknown.invalid", serverName: "app.example.com", status: http.StatusMisdirectedRequest},
				{name: "both unmatched", host: "unknown.invalid", serverName: "missing.invalid", status: http.StatusNotFound},
			} {
				t.Run(tc.name, func(t *testing.T) {
					resp, conn, err := doHTTPSRequestWithServerName(listenAddr, tc.host, tc.serverName, insecure)
					require.NoError(t, err)
					defer func() { _ = conn.Close() }()
					defer func() { _ = resp.Body.Close() }()
					require.Equal(t, tc.status, resp.StatusCode)
					if tc.body != "" {
						body, err := io.ReadAll(resp.Body)
						require.NoError(t, err)
						require.Equal(t, tc.body, string(body))
					}
				})
			}
		})
	}
}
