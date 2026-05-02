package entrypoint

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	agentcert "github.com/yusing/godoxy/agent/pkg/agent"
	"github.com/yusing/godoxy/internal/agentpool"
	autocert "github.com/yusing/godoxy/internal/autocert/types"
	"github.com/yusing/godoxy/internal/common"
	"github.com/yusing/godoxy/internal/homepage"
	netutils "github.com/yusing/godoxy/internal/net"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	routetypes "github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

func TestSNIMatchUsesAliasOrFQDN(t *testing.T) {
	routes := map[string]bool{
		"site2":             true,
		"fqdn.example.test": true,
	}
	exists := func(key string) bool { return routes[key] }

	key, ok := matchSNI("site2.example.test", findRouteKeyAnyDomain, exists)
	require.True(t, ok)
	require.Equal(t, "site2", key)

	key, ok = matchSNI("fqdn.example.test", findRouteKeyAnyDomain, exists)
	require.True(t, ok)
	require.Equal(t, "fqdn.example.test", key)

	_, ok = matchSNI("other.example.test", findRouteKeyAnyDomain, exists)
	require.False(t, ok)
}

func TestSNIMatchHonorsDomainFilters(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	ep.SetFindRouteDomains([]string{".example.test"})
	exists := func(key string) bool { return key == "db" }

	key, ok := matchSNI("db.example.test", ep.findRouteKeyFunc, exists)
	require.True(t, ok)
	require.Equal(t, "db", key)

	_, ok = matchSNI("db.attacker.test", ep.findRouteKeyFunc, exists)
	require.False(t, ok)
}

func TestSNIRouterListensPerHTTPSAddress(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() {
		require.NoError(t, ep.sni.Close())
	})

	addr1 := reserveSNIListenAddr(t)
	addr2 := reserveSNIListenAddr(t)

	listener1, err := ep.sni.Listen(addr1)
	require.NoError(t, err)
	listener2, err := ep.sni.Listen(addr2)
	require.NoError(t, err)

	require.NotSame(t, listener1, listener2)
	require.Equal(t, addr1, listener1.Addr().String())
	require.Equal(t, addr2, listener2.Addr().String())
	require.Equal(t, 2, ep.sni.listeners.Size())

	again, err := ep.sni.Listen(addr1)
	require.NoError(t, err)
	require.Same(t, listener1, again)
}

func TestSharedHTTPSListenAddrMatchesConfiguredWildcard(t *testing.T) {
	port := strconv.Itoa(common.ProxyHTTPSPort)
	require.True(t, netutils.IsSharedHTTPSListenAddr(common.ProxyHTTPSAddr))

	isConfiguredWildcard := netutils.IsWildcardListenHost(common.ProxyHTTPSAddr)
	for _, addr := range []string{
		net.JoinHostPort("0.0.0.0", port),
		net.JoinHostPort("::", port),
		net.JoinHostPort("0:0:0:0:0:0:0:0", port),
	} {
		a := addr
		t.Run(a, func(t *testing.T) {
			require.Equal(t, isConfiguredWildcard, netutils.IsSharedHTTPSListenAddr(a), a)
			require.True(t, netutils.IsWildcardListenHost(a), a)
		})
	}

	otherPort := "1"
	if port == otherPort {
		otherPort = "2"
	}
	require.False(t, netutils.IsSharedHTTPSListenAddr(net.JoinHostPort("0.0.0.0", otherPort)))
	require.False(t, netutils.IsWildcardListenHost(net.JoinHostPort("127.0.0.1", port)))
}

func TestSNIListenerQueuesHTTPSBurst(t *testing.T) {
	const burst = 129

	listener := &sniListener{
		https: make(chan net.Conn, httpsForwardBacklog),
	}

	conns := make([]*trackingConn, burst)
	for i := range conns {
		conns[i] = &trackingConn{}
		listener.forwardHTTPS(conns[i])
	}

	for i, conn := range conns {
		require.False(t, conn.closed, "connection %d was dropped before HTTPS listener accepted it", i)
	}
	require.Equal(t, burst, len(listener.https))
}

func TestHTTPSListenBypassesSNIRouterWhenTCPRoutingDisabled(t *testing.T) {
	ca, srv, _, err := agentcert.NewAgent()
	require.NoError(t, err)
	_ = ca

	serverCert, err := srv.ToTLSCert()
	require.NoError(t, err)

	ep := NewTestEntrypoint(t, nil)
	t.Cleanup(func() {
		closeTestServers(t, ep)
	})
	autocert.SetCtx(task.GetTestTask(t), &staticCertProvider{cert: serverCert})

	prev := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = false
	t.Cleanup(func() {
		common.SNIRoutingForTCPRoutes = prev
	})

	listener, releaseListener := reserveTCPAddr(t)
	listenAddr := listener.Addr().String()
	addHTTPRouteAt(t, ep, "app", "", listenAddr, listener)
	releaseListener()

	require.Equal(t, 0, ep.sni.listeners.Size())

	resp, err := doHTTPSRequest(listenAddr, "app.example.com", &tls.Config{
		InsecureSkipVerify: true,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()
}

func TestSharedHTTPSTCPRouteRejectedWhenSNIRoutingDisabled(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)

	prev := common.SNIRoutingForTCPRoutes
	common.SNIRoutingForTCPRoutes = false
	t.Cleanup(func() {
		common.SNIRoutingForTCPRoutes = prev
	})

	err := ep.StartAddRoute(&fakeStreamRoute{
		name:      "tcp-shared-https",
		listenURL: nettypes.MustParseURL("tcp://" + common.ProxyHTTPSAddr),
		stream:    &fakeStream{},
		task:      task.GetTestTask(t),
	})
	require.ErrorContains(t, err, "TCP SNI routing is disabled")
}

func TestReadClientHelloServerNameReplaysBytes(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientErr := make(chan error, 1)
	go func() {
		client := tls.Client(clientConn, &tls.Config{
			ServerName:         "WWW.Site2.Domain.TLD",
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		clientErr <- client.Handshake()
	}()

	serverName, replayConn, err := readClientHelloServerName(serverConn)
	require.NoError(t, err)
	require.Equal(t, "www.site2.domain.tld", serverName)

	_ = replayConn.SetReadDeadline(time.Now().Add(time.Second))
	first := make([]byte, 1)
	_, err = io.ReadFull(replayConn, first)
	require.NoError(t, err)
	require.Equal(t, byte(0x16), first[0])

	clientConn.Close()
	require.Error(t, <-clientErr)
}

type trackingConn struct {
	closed bool
}

func (c *trackingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *trackingConn) Write(p []byte) (int, error)      { return len(p), nil }
func (c *trackingConn) Close() error                     { c.closed = true; return nil }
func (c *trackingConn) LocalAddr() net.Addr              { return dummyAddr("local") }
func (c *trackingConn) RemoteAddr() net.Addr             { return dummyAddr("remote") }
func (c *trackingConn) SetDeadline(time.Time) error      { return nil }
func (c *trackingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *trackingConn) SetWriteDeadline(time.Time) error { return nil }

type dummyAddr string

func (a dummyAddr) Network() string { return string(a) }
func (a dummyAddr) String() string  { return string(a) }

type fakeStreamRoute struct {
	name      string
	listenURL *nettypes.URL
	stream    *fakeStream
	task      *task.Task
}

func (r *fakeStreamRoute) Key() string             { return r.name }
func (r *fakeStreamRoute) Name() string            { return r.name }
func (r *fakeStreamRoute) Start(task.Parent) error { return nil }
func (r *fakeStreamRoute) Task() *task.Task        { return r.task }
func (r *fakeStreamRoute) Finish(any)              {}
func (r *fakeStreamRoute) MarshalZerologObject(*zerolog.Event) {
}
func (r *fakeStreamRoute) ProviderName() string                  { return "" }
func (r *fakeStreamRoute) GetProvider() routetypes.RouteProvider { return nil }
func (r *fakeStreamRoute) ListenURL() *nettypes.URL              { return r.listenURL }
func (r *fakeStreamRoute) TargetURL() *nettypes.URL              { return nil }
func (r *fakeStreamRoute) HealthMonitor() routetypes.HealthMonitor {
	return nil
}
func (r *fakeStreamRoute) SetHealthMonitor(routetypes.HealthMonitor) {
}
func (r *fakeStreamRoute) References() []string { return nil }
func (r *fakeStreamRoute) ShouldExclude() bool  { return false }
func (r *fakeStreamRoute) Started() <-chan struct{} {
	return nil
}
func (r *fakeStreamRoute) IdlewatcherConfig() *routetypes.IdlewatcherConfig {
	return nil
}
func (r *fakeStreamRoute) HealthCheckConfig() routetypes.HealthCheckConfig {
	return routetypes.HealthCheckConfig{}
}
func (r *fakeStreamRoute) LoadBalanceConfig() *routetypes.LoadBalancerConfig {
	return nil
}
func (r *fakeStreamRoute) HomepageItem() homepage.Item { return homepage.Item{} }
func (r *fakeStreamRoute) DisplayName() string         { return r.name }
func (r *fakeStreamRoute) ContainerInfo() *routetypes.Container {
	return nil
}
func (r *fakeStreamRoute) InboundMTLSProfileRef() string { return "" }
func (r *fakeStreamRoute) RouteMiddlewares() map[string]routetypes.LabelMap {
	return nil
}
func (r *fakeStreamRoute) GetAgent() *agentpool.Agent { return nil }
func (r *fakeStreamRoute) IsDocker() bool             { return false }
func (r *fakeStreamRoute) IsAgent() bool              { return false }
func (r *fakeStreamRoute) UseLoadBalance() bool       { return false }
func (r *fakeStreamRoute) UseIdleWatcher() bool       { return false }
func (r *fakeStreamRoute) UseHealthCheck() bool       { return false }
func (r *fakeStreamRoute) UseAccessLog() bool         { return false }
func (r *fakeStreamRoute) Stream() nettypes.Stream    { return r.stream }
func (r *fakeStreamRoute) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) error {
	return r.stream.ListenAndServe(ctx, preDial, onRead)
}
func (r *fakeStreamRoute) LocalAddr() net.Addr { return r.stream.LocalAddr() }
func (r *fakeStreamRoute) Close() error        { return r.stream.Close() }

type fakeStream struct{}

func (s *fakeStream) ListenAndServe(context.Context, nettypes.HookFunc, nettypes.HookFunc) error {
	return nil
}
func (s *fakeStream) LocalAddr() net.Addr { return dummyAddr("stream") }
func (s *fakeStream) Close() error        { return nil }

func reserveSNIListenAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}
