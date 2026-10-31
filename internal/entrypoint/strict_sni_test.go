package entrypoint

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/goutils/task"
)

func TestResolveRequestRouteStrictSNI(t *testing.T) {
	newServer := func(t *testing.T, strict bool) *httpServer {
		t.Helper()
		ep := NewTestEntrypoint(t, &Config{StrictSNI: strict})
		ep.SetFindRouteDomains([]string{".example.com"})
		srv := newTestHTTPServer(t, ep)
		srv.AddRoute(newFakeHTTPRoute(t, "app", ""))
		srv.AddRoute(newFakeHTTPRoute(t, "other", ""))
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
		{name: "no sni", host: "app.example.com", tls: &tls.ConnectionState{}, wantErr: errStrictSNIRequired},
		{name: "sni for another route", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "other.example.com"}, wantErr: errStrictSNIMisdirected},
		{name: "sni matches no route", host: "app.example.com", tls: &tls.ConnectionState{ServerName: "unknown.example.com"}, wantErr: errStrictSNIMisdirected},
		{name: "unknown host and sni fall through", host: "unknown.example.com", tls: &tls.ConnectionState{ServerName: "unknown.example.com"}},
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
	_, srv, _, err := agentcert.NewAgent()
	require.NoError(t, err)
	serverCert, err := srv.ToTLSCert()
	require.NoError(t, err)

	ep := NewTestEntrypoint(t, &Config{StrictSNI: true})
	t.Cleanup(func() {
		closeTestServers(t, ep)
	})
	ep.SetFindRouteDomains([]string{".example.com"})
	autocert.SetCtx(task.GetTestTask(t), &staticCertProvider{cert: serverCert})

	listener, releaseListener := reserveTCPAddr(t)
	listenAddr := listener.Addr().String()
	addHTTPRouteAt(t, ep, "app", "", listenAddr, listener)
	releaseListener()
	addHTTPRouteAt(t, ep, "other", "", listenAddr, nil)

	insecure := &tls.Config{InsecureSkipVerify: true}

	t.Run("matching sni succeeds", func(t *testing.T) {
		resp, err := doHTTPSRequest(listenAddr, "app.example.com", insecure)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	for _, tc := range []struct{ name, host, serverName string }{
		{name: "no sni", host: "app.example.com", serverName: ""},
		{name: "ip as sni", host: "app.example.com", serverName: "127.0.0.1"},
		{name: "sni for another route", host: "app.example.com", serverName: "other.example.com"},
	} {
		t.Run(tc.name+" is misdirected", func(t *testing.T) {
			resp, conn, err := doHTTPSRequestWithServerName(listenAddr, tc.host, tc.serverName, insecure)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusMisdirectedRequest, resp.StatusCode)
		})
	}
}
