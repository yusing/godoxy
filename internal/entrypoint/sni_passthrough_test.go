package entrypoint

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/stretchr/testify/require"
	"github.com/yusing/godoxy/internal/common"
	nettypes "github.com/yusing/godoxy/internal/net/types"
)

type noopConnProxy struct{}

func (noopConnProxy) ProxyConn(context.Context, net.Conn) {}

type contextRecordingProxy struct {
	ctxCh chan context.Context
}

type fakeStream struct{}

func (fakeStream) ListenAndServe(context.Context, nettypes.HookFunc, nettypes.HookFunc) error {
	return nil
}
func (fakeStream) LocalAddr() net.Addr { return &net.TCPAddr{} }
func (fakeStream) Close() error        { return nil }

type fakeSNIStreamRoute struct {
	*fakeHTTPRoute
	stream nettypes.Stream
}

func newFakeSNIStreamRoute(t *testing.T, alias, listenURL string, stream nettypes.Stream) *fakeSNIStreamRoute {
	t.Helper()

	return &fakeSNIStreamRoute{
		fakeHTTPRoute: newFakeHTTPRouteAt(t, alias, "", listenURL),
		stream:        stream,
	}
}

func (r *fakeSNIStreamRoute) Stream() nettypes.Stream { return r.stream }
func (r *fakeSNIStreamRoute) ListenAndServe(ctx context.Context, preDial, onRead nettypes.HookFunc) error {
	return r.stream.ListenAndServe(ctx, preDial, onRead)
}
func (r *fakeSNIStreamRoute) LocalAddr() net.Addr { return r.stream.LocalAddr() }
func (r *fakeSNIStreamRoute) Close() error        { return r.stream.Close() }

type trackingConn struct {
	closed bool
}

func (c *trackingConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *trackingConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *trackingConn) Close() error                     { c.closed = true; return nil }
func (c *trackingConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *trackingConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *trackingConn) SetDeadline(time.Time) error      { return nil }
func (c *trackingConn) SetReadDeadline(time.Time) error  { return nil }
func (c *trackingConn) SetWriteDeadline(time.Time) error { return nil }

func (p *contextRecordingProxy) ProxyConn(ctx context.Context, conn net.Conn) {
	p.ctxCh <- ctx
	<-ctx.Done()
	_ = conn.Close()
}

func TestSNIListenKeyPreservesExplicitBindNetwork(t *testing.T) {
	defaultKey := newSNIListenKeyForAddr(":443")
	explicitIPv4Key := newSNIListenKey("tcp4", "0.0.0.0:443")

	require.NotEqual(t, defaultKey.String(), explicitIPv4Key.String())
	require.Equal(t, explicitIPv4Key, newSNIListenKeyForAddr("0.0.0.0:443"))
	require.Equal(t, newSNIListenKey("tcp6", "[::]:443"), newSNIListenKeyForAddr("[::]:443"))
}

func TestSNIPassthroughMuxUsesRouteContextForMatchedProxy(t *testing.T) {
	routeCtx, cancelRoute := context.WithCancel(t.Context())
	defer cancelRoute()

	proxy := &contextRecordingProxy{ctxCh: make(chan context.Context, 1)}
	mux := &sniPassthroughMux{
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
	}
	require.NoError(t, mux.addRoute("site2", proxy, routeCtx))

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		mux.handleConn(t.Context(), serverConn)
	}()

	tlsClient := tls.Client(clientConn, &tls.Config{
		ServerName:         "site2.localhost",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	_ = clientConn.SetDeadline(time.Now().Add(time.Second))
	go func() {
		_ = tlsClient.Handshake()
	}()

	var proxiedCtx context.Context
	select {
	case proxiedCtx = <-proxy.ctxCh:
	case <-time.After(time.Second):
		t.Fatal("matched SNI connection was not proxied")
	}
	require.Equal(t, routeCtx, proxiedCtx)

	cancelRoute()
	require.Eventually(t, func() bool {
		select {
		case <-proxiedCtx.Done():
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)
	<-done
}

func TestSNIPassthroughMuxMatchesAliasLikeHTTPRoutes(t *testing.T) {
	mux := &sniPassthroughMux{
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
	}
	require.NoError(t, mux.addRoute("site2", noopConnProxy{}, t.Context()))
	require.NoError(t, mux.addRoute("fqdn.example.test", noopConnProxy{}, t.Context()))

	target, ok := mux.matchRoute("site2.example.test")
	require.True(t, ok)
	require.Equal(t, "site2", target.key)

	target, ok = mux.matchRoute("fqdn.example.test")
	require.True(t, ok)
	require.Equal(t, "fqdn.example.test", target.key)

	_, ok = mux.matchRoute("other.example.test")
	require.False(t, ok)
}

func TestQueuedListenerEnqueueCloseConcurrentDoesNotPanic(t *testing.T) {
	panicObserved := make(chan any, 1)

	for range 1000 {
		listener := newQueuedListener(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443})
		clientConn, serverConn := net.Pipe()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer clientConn.Close()
			defer serverConn.Close()
			defer func() {
				if recovered := recover(); recovered != nil {
					select {
					case panicObserved <- recovered:
					default:
					}
				}
			}()
			_ = listener.Enqueue(serverConn)
		}()
		_ = listener.Close()
		<-done
		select {
		case recovered := <-panicObserved:
			t.Fatalf("Enqueue panicked while Close ran concurrently: %v", recovered)
		default:
		}
	}
}

func TestQueuedListenerCloseClosesBufferedConnections(t *testing.T) {
	listener := newQueuedListener(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443})
	conn1 := &trackingConn{}
	conn2 := &trackingConn{}

	require.NoError(t, listener.Enqueue(conn1))
	require.NoError(t, listener.Enqueue(conn2))
	require.NoError(t, listener.Close())
	require.True(t, conn1.closed)
	require.True(t, conn2.closed)
}

func TestAsSNIPassthroughRouteOnlyMatchesHTTPSAddr(t *testing.T) {
	httpsRoute := newFakeSNIStreamRoute(t, "site2", "tcp://"+common.ProxyHTTPSAddr, fakeStream{})
	sniRoute, ok := asSNIPassthroughRoute(httpsRoute)
	require.True(t, ok)
	require.Equal(t, httpsRoute, sniRoute)

	normalTCPRoute := newFakeSNIStreamRoute(t, "ssh", "tcp://127.0.0.1:2222", fakeStream{})
	_, ok = asSNIPassthroughRoute(normalTCPRoute)
	require.False(t, ok)
}

func TestSNIPassthroughManagerAddRouteRejectsNonProxyStreamBeforeBinding(t *testing.T) {
	ep := NewTestEntrypoint(t, nil)
	manager := newSNIPassthroughManager(ep)
	route := newFakeSNIStreamRoute(t, "site2", "tcp://"+common.ProxyHTTPSAddr, fakeStream{})

	err := manager.AddRoute(route)
	require.ErrorContains(t, err, `route "site2" stream does not support accepted connection proxying`)

	key := newSNIListenKey(route.ListenURL().Scheme, route.ListenURL().Host)
	mux, ok := manager.muxes.Load(key.String())
	if ok {
		mux.close()
	}
	require.False(t, ok, "AddRoute must not bind the shared SNI listener before validating the proxy stream")
}

func TestSNIPassthroughMuxConcurrentAddsMatchByAlias(t *testing.T) {
	mux := &sniPassthroughMux{
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
		https:  newQueuedListener(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443}),
	}

	const routeCount = 64
	var wg sync.WaitGroup
	errs := make(chan error, routeCount)
	for i := range routeCount {
		wg.Go(func() {
			host := fmt.Sprintf("host-%d.example.test", i)
			errs <- mux.addRoute(host, noopConnProxy{}, t.Context())
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	for i := range routeCount {
		host := fmt.Sprintf("host-%d.example.test", i)
		target, ok := mux.matchRoute(host)
		require.True(t, ok, "matcher missed %s", host)
		require.Equal(t, host, target.key)
	}
}

func TestSNIPassthroughMuxHandleConnTimesOutIdleClientHello(t *testing.T) {
	originalTimeout := clientHelloTimeout
	clientHelloTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		clientHelloTimeout = originalTimeout
	})

	mux := &sniPassthroughMux{
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
		https:  newQueuedListener(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443}),
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		mux.handleConn(t.Context(), serverConn)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("handleConn did not return after client hello timeout")
	}

	queuedConn, err := mux.httpsListener().Accept()
	require.NoError(t, err)
	defer queuedConn.Close()

	writeErr := make(chan error, 1)
	go func() {
		_, err := clientConn.Write([]byte{0x42})
		writeErr <- err
	}()
	require.NoError(t, queuedConn.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	first := make([]byte, 1)
	_, err = io.ReadFull(queuedConn, first)
	require.NoError(t, err)
	require.Equal(t, byte(0x42), first[0])
	require.NoError(t, <-writeErr)
}

func TestSNIPassthroughMuxForwardHTTPSWithoutListenerClosesConn(t *testing.T) {
	base, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = base.Close() })
	mux := &sniPassthroughMux{
		base:   base,
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	mux.forwardHTTPS(serverConn)
	require.Nil(t, mux.https, "unmatched HTTPS fallback must not create an unowned queue")

	_, err = clientConn.Write([]byte{0x42})
	require.Error(t, err)
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
	require.Equal(t, byte(0x16), first[0], "replayed stream starts with TLS handshake record")

	clientConn.Close()
	require.Error(t, <-clientErr)
}
