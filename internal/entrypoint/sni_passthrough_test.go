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
)

type noopConnProxy struct{}

func (noopConnProxy) ProxyConn(context.Context, net.Conn) {}

type contextRecordingProxy struct {
	ctxCh chan context.Context
}

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
	mux.matcher.Store(newSNIPatternMatcher())
	require.NoError(t, mux.addRoute("site2", []string{"site2.localhost"}, proxy, routeCtx))

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

func TestSNIPatternMatcherPrefersExactThenLongestWildcard(t *testing.T) {
	matcher := newSNIPatternMatcher()
	matcher.add("wild-short", []string{"*.domain.tld"})
	matcher.add("wild-long", []string{"*.site2.domain.tld"})
	matcher.add("exact", []string{"api.site2.domain.tld"})

	name, ok := matcher.match("api.site2.domain.tld")
	require.True(t, ok)
	require.Equal(t, "exact", name)

	name, ok = matcher.match("www.site2.domain.tld")
	require.True(t, ok)
	require.Equal(t, "wild-long", name)

	name, ok = matcher.match("other.domain.tld")
	require.True(t, ok)
	require.Equal(t, "wild-short", name)

	name, ok = matcher.match("site2.domain.tld")
	require.True(t, ok, "*.site2.domain.tld bare suffix should fall back to less-specific wildcard")
	require.Equal(t, "wild-short", name)
}

func TestSNIPatternMatcherBareSuffixFallsBackToLessSpecificWildcard(t *testing.T) {
	matcher := newSNIPatternMatcher()
	matcher.add("less-specific", []string{"*.domain.tld"})
	matcher.add("more-specific", []string{"*.site2.domain.tld"})
	name, ok := matcher.match("site2.domain.tld")
	require.True(t, ok)
	require.Equal(t, "less-specific", name)
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

func TestSNIPassthroughMuxConcurrentAddsRebuildMatcherAtomically(t *testing.T) {
	mux := &sniPassthroughMux{
		routes: xsync.NewMap[string, *sniPassthroughTarget](),
		https:  newQueuedListener(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 443}),
	}
	mux.matcher.Store(newSNIPatternMatcher())

	const routeCount = 64
	var wg sync.WaitGroup
	errs := make(chan error, routeCount)
	for i := range routeCount {
		wg.Go(func() {
			host := fmt.Sprintf("host-%d.example.test", i)
			errs <- mux.addRoute(host, []string{host}, noopConnProxy{}, t.Context())
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	matcher := mux.matcher.Load()
	require.NotNil(t, matcher)
	for i := range routeCount {
		host := fmt.Sprintf("host-%d.example.test", i)
		name, ok := matcher.match(host)
		require.True(t, ok, "matcher missed %s", host)
		require.Equal(t, host, name)
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
	mux.matcher.Store(newSNIPatternMatcher())

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
	mux.matcher.Store(newSNIPatternMatcher())

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
