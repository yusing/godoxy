package entrypoint

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/pool"
	"github.com/yusing/goutils/task"
)

type fakeHTTPRoute struct {
	key                string
	name               string
	inboundMTLSProfile string
	listenURL          *nettypes.URL
	task               *task.Task
}

func newFakeHTTPRoute(t *testing.T, alias, profile string) *fakeHTTPRoute {
	return newFakeHTTPRouteAt(t, alias, profile, "https://:1000")
}

func newFakeHTTPRouteAt(t *testing.T, alias, profile, listenURL string) *fakeHTTPRoute {
	t.Helper()

	return &fakeHTTPRoute{
		key:                alias,
		name:               alias,
		inboundMTLSProfile: profile,
		listenURL:          nettypes.MustParseURL(listenURL),
		task:               task.GetTestTask(t),
	}
}

func (r *fakeHTTPRoute) Key() string             { return r.key }
func (r *fakeHTTPRoute) Name() string            { return r.name }
func (r *fakeHTTPRoute) Start(task.Parent) error { return nil }
func (r *fakeHTTPRoute) Task() *task.Task        { return r.task }
func (r *fakeHTTPRoute) Finish(any) {
	// no-op: test stub
}
func (r *fakeHTTPRoute) MarshalZerologObject(*zerolog.Event) {
	// no-op: test stub
}
func (r *fakeHTTPRoute) ProviderName() string               { return "" }
func (r *fakeHTTPRoute) GetProvider() types.RouteProvider   { return nil }
func (r *fakeHTTPRoute) ListenURL() *nettypes.URL           { return r.listenURL }
func (r *fakeHTTPRoute) TargetURL() *nettypes.URL           { return nil }
func (r *fakeHTTPRoute) HealthMonitor() types.HealthMonitor { return nil }
func (r *fakeHTTPRoute) SetHealthMonitor(types.HealthMonitor) {
	// no-op: test stub
}
func (r *fakeHTTPRoute) References() []string                        { return nil }
func (r *fakeHTTPRoute) ShouldExclude() bool                         { return false }
func (r *fakeHTTPRoute) Started() <-chan struct{}                    { return nil }
func (r *fakeHTTPRoute) IdlewatcherConfig() *types.IdlewatcherConfig { return nil }
func (r *fakeHTTPRoute) HealthCheckConfig() types.HealthCheckConfig  { return types.HealthCheckConfig{} }
func (r *fakeHTTPRoute) LoadBalanceConfig() *types.LoadBalancerConfig {
	return nil
}
func (r *fakeHTTPRoute) HomepageItem() homepage.Item { return homepage.Item{} }
func (r *fakeHTTPRoute) DisplayName() string         { return r.name }
func (r *fakeHTTPRoute) ContainerInfo() *types.Container {
	return nil
}
func (r *fakeHTTPRoute) GetAgent() *agentpool.Agent { return nil }
func (r *fakeHTTPRoute) IsDocker() bool             { return false }
func (r *fakeHTTPRoute) IsAgent() bool              { return false }
func (r *fakeHTTPRoute) UseLoadBalance() bool       { return false }
func (r *fakeHTTPRoute) UseIdleWatcher() bool       { return false }
func (r *fakeHTTPRoute) UseHealthCheck() bool       { return false }
func (r *fakeHTTPRoute) UseAccessLog() bool         { return false }
func (r *fakeHTTPRoute) ServeHTTP(http.ResponseWriter, *http.Request) {
	// no-op: test stub
}
func (r *fakeHTTPRoute) InboundMTLSProfileRef() string { return r.inboundMTLSProfile }

func newTestHTTPServer(t *testing.T, ep *Entrypoint) *httpServer {
	t.Helper()

	srv, ok := ep.servers.Load(common.ProxyHTTPAddr)
	if ok {
		return srv
	}

	srv = &httpServer{
		ep:     ep,
		addr:   common.ProxyHTTPAddr,
		routes: pool.New[types.HTTPRoute]("test-http-routes", "test-http-routes"),
	}
	ep.servers.Store(common.ProxyHTTPAddr, srv)
	return srv
}

func TestMutateServerTLSConfigWithGlobalProfile(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{InboundMTLSProfile: "global"})
	srv := newTestHTTPServer(t, ep)
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"global": {UseSystemCAs: true},
	}))

	base := &tls.Config{MinVersion: tls.VersionTLS12}
	mutated := srv.mutateServerTLSConfig(base)

	require.Equal(t, tls.RequireAndVerifyClientCert, mutated.ClientAuth)
	require.NotNil(t, mutated.ClientCAs)
	require.Nil(t, mutated.GetConfigForClient)
}

func TestMutateServerTLSConfigWithRouteProfiles(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.com"})
	srv := newTestHTTPServer(t, ep)
	srv.AddRoute(newFakeHTTPRoute(t, "secure-app", "route"))
	srv.AddRoute(newFakeHTTPRoute(t, "open-app", ""))
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"route": {UseSystemCAs: true},
	}))

	base := &tls.Config{MinVersion: tls.VersionTLS12}
	mutated := srv.mutateServerTLSConfig(base)

	require.Zero(t, mutated.ClientAuth)
	require.Nil(t, mutated.ClientCAs)
	require.NotNil(t, mutated.GetConfigForClient)

	secureCfg, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "secure-app.example.com"})
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, secureCfg.ClientAuth)
	require.NotNil(t, secureCfg.ClientCAs)
	require.Nil(t, secureCfg.GetConfigForClient)

	openCfg, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "open-app.example.com"})
	require.NoError(t, err)
	require.Zero(t, openCfg.ClientAuth)
	require.Nil(t, openCfg.ClientCAs)
	require.Nil(t, openCfg.GetConfigForClient)

	unknownCfg, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "unknown.example.com"})
	require.NoError(t, err)
	require.Zero(t, unknownCfg.ClientAuth)
	require.Nil(t, unknownCfg.ClientCAs)
	require.Nil(t, unknownCfg.GetConfigForClient)
}

func TestMutateServerTLSConfigFallsBackToRouteProfilesAfterGlobalLookupError(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{InboundMTLSProfile: "missing"})
	ep.SetFindRouteDomains([]string{".example.com"})
	srv := newTestHTTPServer(t, ep)
	srv.AddRoute(newFakeHTTPRoute(t, "secure-app", "route"))
	ep.inboundMTLSProfiles = map[string]*x509.CertPool{
		"route": x509.NewCertPool(),
	}

	base := &tls.Config{MinVersion: tls.VersionTLS12}
	mutated := srv.mutateServerTLSConfig(base)

	require.NotNil(t, mutated.GetConfigForClient)

	secureCfg, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "secure-app.example.com"})
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, secureCfg.ClientAuth)
	require.NotNil(t, secureCfg.ClientCAs)
	require.Nil(t, secureCfg.GetConfigForClient)

	openCfg, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "open-app.example.com"})
	require.NoError(t, err)
	require.Zero(t, openCfg.ClientAuth)
	require.Nil(t, openCfg.ClientCAs)
	require.Nil(t, openCfg.GetConfigForClient)
}

func TestSetInboundMTLSProfilesRejectsUnknownGlobalProfile(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{InboundMTLSProfile: "missing"})
	err := ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"known": {UseSystemCAs: true},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, `entrypoint inbound mTLS profile "missing" not found`)
}

func TestSetInboundMTLSProfilesRejectsBadCAFile(t *testing.T) {
	ep := NewTestEntrypoint(t, &Config{InboundMTLSProfile: "broken"})
	err := ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"broken": {CAFiles: []string{filepath.Join(t.TempDir(), "missing.pem")}},
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "missing.pem")
}

func TestCompileInboundMTLSProfilesReturnsNilMapOnError(t *testing.T) {
	compiled, err := compileInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"ok":  {UseSystemCAs: true},
		"bad": {CAFiles: []string{filepath.Join(t.TempDir(), "missing.pem")}},
	})
	require.Nil(t, compiled)
	require.Error(t, err)
	require.ErrorContains(t, err, "missing.pem")
}

func TestMutateServerTLSConfigRejectsUnknownRouteProfile(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.com"})
	srv := newTestHTTPServer(t, ep)
	srv.AddRoute(newFakeHTTPRoute(t, "secure-app", "missing"))

	base := &tls.Config{MinVersion: tls.VersionTLS12}
	mutated := srv.mutateServerTLSConfig(base)

	_, err := mutated.GetConfigForClient(&tls.ClientHelloInfo{ServerName: "secure-app.example.com"})
	require.Error(t, err)
	require.ErrorContains(t, err, `route "secure-app" inbound mTLS profile "missing" not found`)
}

func TestResolveRequestRouteRejectsUnknownRouteProfile(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.com"})
	srv := newTestHTTPServer(t, ep)
	srv.AddRoute(newFakeHTTPRoute(t, "secure-app", "missing"))

	req := httptest.NewRequest(http.MethodGet, "https://secure-app.example.com", nil)
	req.Host = "secure-app.example.com"
	req.TLS = &tls.ConnectionState{ServerName: "secure-app.example.com"}

	route, err := srv.resolveRequestRoute(req)
	require.Nil(t, route)
	require.Error(t, err)
	require.ErrorContains(t, err, `route "secure-app" inbound mTLS profile "missing" not found`)
}

func TestInboundMTLSGlobalHandshake(t *testing.T) {
	ca, srv, client, err := agentcert.NewAgent()
	require.NoError(t, err)

	serverCert, err := srv.ToTLSCert()
	require.NoError(t, err)
	clientCert, err := client.ToTLSCert()
	require.NoError(t, err)

	caPath := writeTempFile(t, "ca.pem", ca.Cert)
	provider := &staticCertProvider{cert: serverCert}

	ep := NewTestEntrypoint(t, &Config{InboundMTLSProfile: "global"})
	t.Cleanup(func() {
		closeTestServers(t, ep)
	})
	autocert.SetCtx(task.GetTestTask(t), provider)
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"global": {CAFiles: []string{caPath}},
	}))

	listener, releaseListener := reserveTCPAddr(t)
	listenAddr := listener.Addr().String()
	addHTTPRouteAt(t, ep, "app1", "", listenAddr, listener)
	releaseListener()

	t.Run("trusted client succeeds", func(t *testing.T) {
		resp, err := doHTTPSRequest(listenAddr, "app1.example.com", &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*clientCert},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})

	t.Run("missing client cert fails handshake", func(t *testing.T) {
		_, err := doHTTPSRequest(listenAddr, "app1.example.com", &tls.Config{
			InsecureSkipVerify: true,
		})
		require.Error(t, err)
	})

	t.Run("wrong client cert fails handshake", func(t *testing.T) {
		_, _, badClient, err := agentcert.NewAgent()
		require.NoError(t, err)
		badClientCert, err := badClient.ToTLSCert()
		require.NoError(t, err)

		_, err = doHTTPSRequest(listenAddr, "app1.example.com", &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*badClientCert},
		})
		require.Error(t, err)
	})
}

func TestInboundMTLSRouteScopedHandshake(t *testing.T) {
	ca, srv, client, err := agentcert.NewAgent()
	require.NoError(t, err)

	serverCert, err := srv.ToTLSCert()
	require.NoError(t, err)
	clientCert, err := client.ToTLSCert()
	require.NoError(t, err)

	caPath := writeTempFile(t, "ca.pem", ca.Cert)
	provider := &staticCertProvider{cert: serverCert}

	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() {
		closeTestServers(t, ep)
	})
	ep.SetFindRouteDomains([]string{".example.com"})
	autocert.SetCtx(task.GetTestTask(t), provider)
	require.NoError(t, ep.SetInboundMTLSProfiles(map[string]types.InboundMTLSProfile{
		"route": {CAFiles: []string{caPath}},
	}))

	listener, releaseListener := reserveTCPAddr(t)
	listenAddr := listener.Addr().String()
	addHTTPRouteAt(t, ep, "secure-app", "route", listenAddr, listener)
	releaseListener()
	addHTTPRouteAt(t, ep, "open-app", "", listenAddr, nil)

	t.Run("secure route requires client cert when sni matches", func(t *testing.T) {
		_, err := doHTTPSRequest(listenAddr, "secure-app.example.com", &tls.Config{
			InsecureSkipVerify: true,
		})
		require.Error(t, err)
	})

	t.Run("secure route accepts trusted client cert", func(t *testing.T) {
		resp, err := doHTTPSRequest(listenAddr, "secure-app.example.com", &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*clientCert},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})

	t.Run("open route without client cert succeeds", func(t *testing.T) {
		resp, err := doHTTPSRequest(listenAddr, "open-app.example.com", &tls.Config{
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	})

	t.Run("secure route rejects requests without sni", func(t *testing.T) {
		resp, tlsConn, err := doHTTPSRequestWithServerName(listenAddr, "secure-app.example.com", "", &tls.Config{
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		defer func() { _ = tlsConn.Close() }()
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusMisdirectedRequest, resp.StatusCode)
	})

	t.Run("secure route rejects host and sni mismatch without cert", func(t *testing.T) {
		resp, tlsConn, err := doHTTPSRequestWithServerName(listenAddr, "secure-app.example.com", "open-app.example.com", &tls.Config{
			InsecureSkipVerify: true,
		})
		require.NoError(t, err)
		defer func() { _ = tlsConn.Close() }()
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusMisdirectedRequest, resp.StatusCode)
	})

	t.Run("open route rejects host and sni mismatch when sni selects secure route", func(t *testing.T) {
		resp, tlsConn, err := doHTTPSRequestWithServerName(listenAddr, "open-app.example.com", "secure-app.example.com", &tls.Config{
			InsecureSkipVerify: true,
			Certificates:       []tls.Certificate{*clientCert},
		})
		require.NoError(t, err)
		defer func() { _ = tlsConn.Close() }()
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusMisdirectedRequest, resp.StatusCode)
	})
}

func addHTTPRouteAt(t *testing.T, ep *Entrypoint, alias, profile, listenAddr string, listener net.Listener) {
	t.Helper()

	route := newFakeHTTPRouteAt(t, alias, profile, "https://"+listenAddr)
	if listener == nil {
		require.NoError(t, ep.StartAddRoute(route))
		return
	}
	require.NoError(t, ep.addHTTPRouteWithListener(route, listenAddr, HTTPProtoHTTPS, listener))
}

func closeTestServers(t *testing.T, ep *Entrypoint) {
	t.Helper()
	for _, srv := range ep.servers.Range {
		srv.Close()
	}
}

func reserveTCPAddr(t *testing.T) (net.Listener, func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	owned := true
	t.Cleanup(func() {
		if owned {
			_ = ln.Close()
		}
	})
	return ln, func() {
		owned = false
	}
}

func writeTempFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func doHTTPSRequest(addr, host string, tlsConfig *tls.Config) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, "https://"+addr, nil)
	if err != nil {
		return nil, err
	}
	req.Host = host

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: cloneTLSConfigWithServerName(tlsConfig, host),
		},
	}
	return client.Do(req)
}

// doHTTPSRequestWithServerName sends GET https://addr/ with HTTP Host set to host and TLS
// ServerName set to serverName (SNI may differ from Host). The returned connection stays open
// until the caller closes it after finishing with resp (typically close resp.Body first, then
// the tls connection).
func doHTTPSRequestWithServerName(addr, host, serverName string, tlsConfig *tls.Config) (*http.Response, io.Closer, error) {
	conn, err := tls.Dial("tcp", addr, cloneTLSConfigWithServerName(tlsConfig, serverName))
	if err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+addr, nil)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	req.Host = host
	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return resp, conn, nil
}

func cloneTLSConfigWithServerName(cfg *tls.Config, serverName string) *tls.Config {
	if cfg == nil {
		cfg = &tls.Config{}
	}
	cloned := cfg.Clone()
	cloned.ServerName = serverName
	return cloned
}

type staticCertProvider struct {
	cert *tls.Certificate
}

func (p *staticCertProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return p.cert, nil
}
func (p *staticCertProvider) GetCertInfos() ([]autocert.CertInfo, error) { return nil, nil }
func (p *staticCertProvider) ScheduleRenewalAll(task.Parent) {
	// no-op: test stub
}
func (p *staticCertProvider) ObtainCertAll() error                 { return nil }
func (p *staticCertProvider) ForceExpiryAll() bool                 { return false }
func (p *staticCertProvider) WaitRenewalDone(context.Context) bool { return true }
