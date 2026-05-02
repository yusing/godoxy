package entrypoint

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/agentpool"
	"github.com/yusing/godoxy/internal/homepage"
	nettypes "github.com/yusing/godoxy/internal/net/types"
	"github.com/yusing/godoxy/internal/types"
	"github.com/yusing/goutils/task"
)

func TestSNIRouterHTTPPathSkipsClientHelloWhenNoSNIRoutes(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	listener, err := ep.sni.Listen(t.Context(), reserveSNIListenAddr(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer clientConn.Close()

	accepted, err := listener.Accept()
	require.NoError(t, err)
	defer accepted.Close()

	writeErr := make(chan error, 1)
	go func() {
		_ = clientConn.SetWriteDeadline(time.Now().Add(time.Second))
		_, err := clientConn.Write([]byte("GET"))
		writeErr <- err
	}()

	_ = accepted.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 3)
	_, err = io.ReadFull(accepted, buf)
	require.NoError(t, <-writeErr)
	require.NoError(t, err)
	require.Equal(t, "GET", string(buf))
}

func TestSNIRouterPendingHTTPSAcceptDoesNotStealFirstSNIRouteConn(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	listener, err := ep.sni.Listen(t.Context(), reserveSNIListenAddr(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, listener.Close()) })

	acceptedCh := make(chan struct {
		conn net.Conn
		err  error
	}, 1)
	go func() {
		conn, err := listener.Accept()
		acceptedCh <- struct {
			conn net.Conn
			err  error
		}{conn: conn, err: err}
	}()

	proxyCalled := make(chan net.Conn, 1)
	route := newFakeSNIStreamRoute(t, "tcp-app", listener.Addr().String(), &testConnProxyStream{
		proxyConn: func(conn net.Conn) {
			proxyCalled <- conn
			_ = conn.Close()
		},
	})
	require.NoError(t, ep.sni.AddRoute(route))

	clientErr := make(chan error, 1)
	go func() {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			clientErr <- err
			return
		}
		defer conn.Close()

		client := tls.Client(conn, &tls.Config{
			ServerName:         "tcp-app.example.test",
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS12,
		})
		clientErr <- client.Handshake()
	}()

	select {
	case conn := <-proxyCalled:
		require.NotNil(t, conn)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SNI route proxy")
	}

	select {
	case accepted := <-acceptedCh:
		if accepted.conn != nil {
			_ = accepted.conn.Close()
		}
		t.Fatalf("pending HTTPS accept stole SNI connection: err=%v", accepted.err)
	case <-time.After(200 * time.Millisecond):
	}

	require.Error(t, <-clientErr)
}

func TestSNIConnQueueClearsWaitingAfterWake(t *testing.T) {
	var q sniConnQueue
	q.init()
	t.Cleanup(q.close)

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	popResult := make(chan struct {
		conn net.Conn
		ok   bool
	}, 1)
	go func() {
		conn, ok := q.pop()
		popResult <- struct {
			conn net.Conn
			ok   bool
		}{conn: conn, ok: ok}
	}()

	require.Eventually(t, func() bool { return q.waiting.Load() }, time.Second, 10*time.Millisecond)
	require.True(t, q.push(serverConn))

	result := <-popResult
	require.True(t, result.ok)
	require.Same(t, serverConn, result.conn)
	require.False(t, q.waiting.Load())
	require.NoError(t, result.conn.Close())
}

func TestSNIConnQueueCloseWakesBlockedPop(t *testing.T) {
	var q sniConnQueue
	q.init()

	popResult := make(chan struct {
		conn net.Conn
		ok   bool
	}, 1)
	go func() {
		conn, ok := q.pop()
		popResult <- struct {
			conn net.Conn
			ok   bool
		}{conn: conn, ok: ok}
	}()

	require.Eventually(t, func() bool { return q.waiting.Load() }, time.Second, 10*time.Millisecond)
	q.close()

	result := <-popResult
	require.False(t, result.ok)
	require.Nil(t, result.conn)
	require.False(t, q.waiting.Load())
}

type fakeSNIStreamRoute struct {
	key       string
	name      string
	listenURL *nettypes.URL
	stream    nettypes.Stream
	task      *task.Task
}

func newFakeSNIStreamRoute(t *testing.T, alias, listenAddr string, stream nettypes.Stream) *fakeSNIStreamRoute {
	t.Helper()
	return &fakeSNIStreamRoute{
		key:       alias,
		name:      alias,
		listenURL: nettypes.MustParseURL("tcp://" + listenAddr),
		stream:    stream,
		task:      task.GetTestTask(t),
	}
}

func (r *fakeSNIStreamRoute) Key() string                                 { return r.key }
func (r *fakeSNIStreamRoute) Name() string                                { return r.name }
func (r *fakeSNIStreamRoute) Start(task.Parent) error                     { return nil }
func (r *fakeSNIStreamRoute) Task() *task.Task                            { return r.task }
func (r *fakeSNIStreamRoute) Finish(any)                                  {}
func (r *fakeSNIStreamRoute) MarshalZerologObject(*zerolog.Event)         {}
func (r *fakeSNIStreamRoute) ProviderName() string                        { return "" }
func (r *fakeSNIStreamRoute) GetProvider() types.RouteProvider            { return nil }
func (r *fakeSNIStreamRoute) ListenURL() *nettypes.URL                    { return r.listenURL }
func (r *fakeSNIStreamRoute) TargetURL() *nettypes.URL                    { return nil }
func (r *fakeSNIStreamRoute) HealthMonitor() types.HealthMonitor          { return nil }
func (r *fakeSNIStreamRoute) SetHealthMonitor(types.HealthMonitor)        {}
func (r *fakeSNIStreamRoute) References() []string                        { return nil }
func (r *fakeSNIStreamRoute) ShouldExclude() bool                         { return false }
func (r *fakeSNIStreamRoute) Started() <-chan struct{}                    { return nil }
func (r *fakeSNIStreamRoute) IdlewatcherConfig() *types.IdlewatcherConfig { return nil }
func (r *fakeSNIStreamRoute) HealthCheckConfig() types.HealthCheckConfig {
	return types.HealthCheckConfig{}
}
func (r *fakeSNIStreamRoute) LoadBalanceConfig() *types.LoadBalancerConfig { return nil }
func (r *fakeSNIStreamRoute) HomepageItem() homepage.Item                  { return homepage.Item{} }
func (r *fakeSNIStreamRoute) DisplayName() string                          { return r.name }
func (r *fakeSNIStreamRoute) ContainerInfo() *types.Container              { return nil }
func (r *fakeSNIStreamRoute) InboundMTLSProfileRef() string                { return "" }
func (r *fakeSNIStreamRoute) RouteMiddlewares() map[string]types.LabelMap  { return nil }
func (r *fakeSNIStreamRoute) GetAgent() *agentpool.Agent                   { return nil }
func (r *fakeSNIStreamRoute) IsDocker() bool                               { return false }
func (r *fakeSNIStreamRoute) IsAgent() bool                                { return false }
func (r *fakeSNIStreamRoute) UseLoadBalance() bool                         { return false }
func (r *fakeSNIStreamRoute) UseIdleWatcher() bool                         { return false }
func (r *fakeSNIStreamRoute) UseHealthCheck() bool                         { return false }
func (r *fakeSNIStreamRoute) UseAccessLog() bool                           { return false }
func (r *fakeSNIStreamRoute) ListenAndServe(context.Context, nettypes.HookFunc, nettypes.HookFunc) error {
	return nil
}
func (r *fakeSNIStreamRoute) LocalAddr() net.Addr     { return nil }
func (r *fakeSNIStreamRoute) Close() error            { return nil }
func (r *fakeSNIStreamRoute) Stream() nettypes.Stream { return r.stream }

type testConnProxyStream struct {
	proxyConn func(net.Conn)
}

var _ nettypes.Stream = (*testConnProxyStream)(nil)
var _ nettypes.ConnProxy = (*testConnProxyStream)(nil)

func (s *testConnProxyStream) ListenAndServe(context.Context, nettypes.HookFunc, nettypes.HookFunc) error {
	return nil
}

func (s *testConnProxyStream) LocalAddr() net.Addr { return nil }
func (s *testConnProxyStream) Close() error        { return nil }

func (s *testConnProxyStream) ProxyConn(_ context.Context, conn net.Conn) {
	if s.proxyConn != nil {
		s.proxyConn(conn)
	}
}
