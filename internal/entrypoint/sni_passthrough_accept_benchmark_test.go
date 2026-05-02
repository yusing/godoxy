package entrypoint

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkSNIMatchRoute(b *testing.B) {
	ep := NewTestEntrypoint(b, nil)
	listener, err := ep.sni.Listen(b.Context(), reserveSNIListenAddr(b))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, listener.Close()) })
	entry := &sniRouteEntry{}
	listener.(*sniListener).routes.Store(&sniRouteTable{byKey: map[string]*sniRouteEntry{"bench": entry}})

	b.ReportAllocs()
	for b.Loop() {
		matched, ok := listener.(*sniListener).match("bench.example.test", ep.findRouteKeyFunc)
		if !ok || matched != entry {
			b.Fatalf("route did not match")
		}
	}
}

func BenchmarkTCPAcceptDirect(b *testing.B) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, listener.Close()) })

	b.ReportAllocs()
	for b.Loop() {
		clientConn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(b, err)
		serverConn, err := listener.Accept()
		require.NoError(b, err)
		require.NoError(b, serverConn.Close())
		require.NoError(b, clientConn.Close())
	}
}

func BenchmarkTCPAcceptWithSNIRouterNoRoutes(b *testing.B) {
	ep := NewTestEntrypoint(b, nil)
	listener, err := ep.sni.Listen(b.Context(), reserveSNIListenAddr(b))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, listener.Close()) })

	b.ReportAllocs()
	for b.Loop() {
		clientConn, err := net.Dial("tcp", listener.Addr().String())
		require.NoError(b, err)
		serverConn, err := listener.Accept()
		require.NoError(b, err)
		require.NoError(b, serverConn.Close())
		require.NoError(b, clientConn.Close())
	}
}
